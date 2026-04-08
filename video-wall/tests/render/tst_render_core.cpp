#include <QtTest/QtTest>

#include <cmath>

#include "render/DecodedFrame.h"
#include "render/TileGrid.h"
#include "render/QualityController.h"
#include "render/FrameClock.h"
#include "render/NullDecoderBackend.h"

#include <chrono>
#include <thread>
#include <vector>

using namespace Kaivue::Render;

// ---------------------------------------------------------------------------
// Helper: build a frame for a given tile from the null decoder
// ---------------------------------------------------------------------------
static DecodedFrameRef makeFrame(NullDecoderBackend& dec, TileId id,
                                  int w = 320, int h = 180)
{
    return dec.generateFrame(id, w, h, /*pts=*/0);
}

// ---------------------------------------------------------------------------
// Test class
// ---------------------------------------------------------------------------
class tst_render_core : public QObject {
    Q_OBJECT

private slots:
    void initTestCase();
    void cleanupTestCase();

    // TileGrid tests
    void tileGrid_setAndCount();
    void tileGrid_remove_during_iteration();
    void tileGrid_maxTiles_64();
    void tileGrid_pixelCoverageComputed();

    // QualityController tests
    void qualityController_lowToMedToHigh();
    void qualityController_hysteresis();

    // FrameClock tests
    void frameClock_normalFrames_stay_under_p99();
    void frameClock_slowFrame_emitsSignal();

    // NullDecoderBackend tests
    void nullDecoder_producesValidFrame();
    void nullDecoder_goldenHash_deterministic();
    void nullDecoder_differentTiles_differentHash();

    // Integration: 600-frame loop with TileGrid + NullDecoder + FrameClock
    void integration_600frames_p99_under_16ms();

private:
    NullDecoderBackend m_dec;
};

// ---------------------------------------------------------------------------
// initTestCase / cleanupTestCase
// ---------------------------------------------------------------------------
void tst_render_core::initTestCase()
{
    // NullDecoderBackend does not need a real QWindow
    QVERIFY(m_dec.init(nullptr));
}

void tst_render_core::cleanupTestCase()
{
    m_dec.shutdown();
}

// ---------------------------------------------------------------------------
// TileGrid tests
// ---------------------------------------------------------------------------
void tst_render_core::tileGrid_setAndCount()
{
    TileGrid grid;
    QCOMPARE(grid.tileCount(), 0);

    for (int i = 0; i < 4; ++i) {
        auto frame = makeFrame(m_dec, TileId(static_cast<uint32_t>(i)));
        grid.setTile(TileId(static_cast<uint32_t>(i)), frame,
                     QRectF(i * 100, 0, 100, 100));
    }
    QCOMPARE(grid.tileCount(), 4);

    grid.removeTile(TileId(2));
    QCOMPARE(grid.tileCount(), 3);
    QVERIFY(grid.tileById(TileId(2)) == nullptr);
}

void tst_render_core::tileGrid_remove_during_iteration()
{
    TileGrid grid;
    for (uint32_t i = 0; i < 8; ++i) {
        auto frame = makeFrame(m_dec, TileId(i));
        grid.setTile(TileId(i), frame, QRectF(i * 10, 0, 10, 10));
    }
    QCOMPARE(grid.tileCount(), 8);

    // Remove even-indexed tiles from inside forEach — must not crash or skip
    int visitCount = 0;
    grid.forEach([&](const TileEntry& entry) {
        ++visitCount;
        if (entry.id.value % 2 == 0) {
            grid.removeTile(entry.id);
        }
    });

    // All 8 should have been visited (snapshot semantics)
    QCOMPARE(visitCount, 8);
    // Even tiles removed → 4 remain
    QCOMPARE(grid.tileCount(), 4);
}

void tst_render_core::tileGrid_maxTiles_64()
{
    TileGrid grid;
    for (uint32_t i = 0; i < 64; ++i) {
        auto frame = makeFrame(m_dec, TileId(i));
        grid.setTile(TileId(i), frame, QRectF(0, 0, 30, 17));
    }
    QCOMPARE(grid.tileCount(), 64);

    // setTile with id >= 64 is silently ignored
    auto frame = makeFrame(m_dec, TileId(64));
    grid.setTile(TileId(64), frame, QRectF(0, 0, 10, 10));
    QCOMPARE(grid.tileCount(), 64); // unchanged
}

void tst_render_core::tileGrid_pixelCoverageComputed()
{
    TileGrid grid;
    // 1920x1080 surface, one tile covering half the width and full height
    grid.setSurfaceSize(QSize(1920, 1080));
    auto frame = makeFrame(m_dec, TileId(0));
    grid.setTile(TileId(0), frame, QRectF(0, 0, 960, 1080));

    const TileEntry* e = grid.tileById(TileId(0));
    QVERIFY(e != nullptr);
    // 960*1080 / (1920*1080) = 0.5
    QVERIFY2(std::abs(e->pixelCoverage - 0.5) < 0.001,
             qPrintable(QString("pixelCoverage %1 != 0.5").arg(e->pixelCoverage)));
}

// ---------------------------------------------------------------------------
// QualityController tests
// ---------------------------------------------------------------------------
void tst_render_core::qualityController_lowToMedToHigh()
{
    TileGrid grid;
    QualityController qc;

    grid.setSurfaceSize(QSize(1920, 1080));

    // Start with tiny tile → LOW coverage
    auto frame = makeFrame(m_dec, TileId(0));
    grid.setTile(TileId(0), frame, QRectF(0, 0, 100, 50));  // ~0.24% → LOW
    qc.evaluate(grid);
    QCOMPARE(qc.currentQuality(TileId(0)), Quality::Low);

    // Grow tile to 20% → should transition to MED
    QSignalSpy spy(&qc, &QualityController::qualityHintChanged);
    grid.setTile(TileId(0), frame, QRectF(0, 0, 960, 432)); // ~20% → MED
    qc.evaluate(grid);
    QCOMPARE(qc.currentQuality(TileId(0)), Quality::Med);
    QVERIFY(spy.count() >= 1);

    // Grow tile to 50% → HIGH
    grid.setTile(TileId(0), frame, QRectF(0, 0, 1920, 540)); // ~50% → HIGH
    qc.evaluate(grid);
    QCOMPARE(qc.currentQuality(TileId(0)), Quality::High);
}

void tst_render_core::qualityController_hysteresis()
{
    TileGrid grid;
    QualityController qc;
    grid.setSurfaceSize(QSize(1920, 1080));

    // Bring to HIGH
    auto frame = makeFrame(m_dec, TileId(5));
    grid.setTile(TileId(5), frame, QRectF(0, 0, 1920, 1080)); // 100%
    qc.evaluate(grid);
    QCOMPARE(qc.currentQuality(TileId(5)), Quality::High);

    // Drop to 36% — above kHighToMedFall (35%), should stay HIGH
    grid.setTile(TileId(5), frame, QRectF(0, 0, 1920, 390)); // ~36%
    QSignalSpy spy(&qc, &QualityController::qualityHintChanged);
    qc.evaluate(grid);
    QCOMPARE(qc.currentQuality(TileId(5)), Quality::High);
    QCOMPARE(spy.count(), 0); // no transition

    // Drop to 30% — below kHighToMedFall → MED
    grid.setTile(TileId(5), frame, QRectF(0, 0, 1920, 324)); // ~30%
    qc.evaluate(grid);
    QCOMPARE(qc.currentQuality(TileId(5)), Quality::Med);
}

// ---------------------------------------------------------------------------
// FrameClock tests
// ---------------------------------------------------------------------------
void tst_render_core::frameClock_normalFrames_stay_under_p99()
{
    FrameClock clock(16.0);

    // Drive 600 very fast frames; P99 should be well under 16 ms on any machine
    for (int i = 0; i < 600; ++i) {
        clock.beginFrame();
        // Simulate ~0.1 ms of work
        volatile int sum = 0;
        for (int j = 0; j < 10000; ++j) sum += j;
        Q_UNUSED(sum);
        clock.endFrame();
    }

    QCOMPARE(clock.frameCount(), std::size_t(600));
    QVERIFY(clock.p99Ms() < 16.0);
}

void tst_render_core::frameClock_slowFrame_emitsSignal()
{
    FrameClock clock(16.0);
    QSignalSpy spy(&clock, &FrameClock::frameTimeExceeded);

    // Drive 100 fast frames to prime the window
    for (int i = 0; i < 100; ++i) {
        clock.beginFrame();
        clock.endFrame();
    }

    // Then drive 500 slow frames (sleep 20 ms each) — P99 will exceed 16 ms
    for (int i = 0; i < 500; ++i) {
        clock.beginFrame();
        std::this_thread::sleep_for(std::chrono::milliseconds(20));
        clock.endFrame();
    }

    QVERIFY(spy.count() > 0);
    const double reported = spy.last().at(0).toDouble();
    QVERIFY(reported > 16.0);
}

// ---------------------------------------------------------------------------
// NullDecoderBackend tests
// ---------------------------------------------------------------------------
void tst_render_core::nullDecoder_producesValidFrame()
{
    const auto frame = m_dec.generateFrame(TileId(0), 320, 180, 1000);
    QVERIFY(frame.isValid());
    QCOMPARE(frame.width,  320);
    QCOMPARE(frame.height, 180);
    QCOMPARE(frame.pts,    std::int64_t(1000));
    QVERIFY(frame.payload != nullptr);
}

void tst_render_core::nullDecoder_goldenHash_deterministic()
{
    // Same tile, same dimensions → same hash every time
    m_dec.generateFrame(TileId(7), 320, 180, 0);
    const uint32_t h1 = m_dec.pixelHash(TileId(7));
    QVERIFY(h1 != 0);

    // Re-generate; dimensions unchanged → buffer unchanged → same hash
    m_dec.generateFrame(TileId(7), 320, 180, 9999); // pts differs — buffer not refilled
    const uint32_t h2 = m_dec.pixelHash(TileId(7));
    QCOMPARE(h1, h2);
}

void tst_render_core::nullDecoder_differentTiles_differentHash()
{
    m_dec.generateFrame(TileId(1), 320, 180, 0);
    m_dec.generateFrame(TileId(2), 320, 180, 0);

    const uint32_t h1 = m_dec.pixelHash(TileId(1));
    const uint32_t h2 = m_dec.pixelHash(TileId(2));

    // Different tiles get different colours → different hashes
    QVERIFY(h1 != h2);
}

// ---------------------------------------------------------------------------
// Integration: 600-frame render loop P99 < 16 ms (no real GPU)
// ---------------------------------------------------------------------------
void tst_render_core::integration_600frames_p99_under_16ms()
{
    NullDecoderBackend dec;
    QVERIFY(dec.init(nullptr));

    TileGrid grid;
    QualityController qc;
    FrameClock clock(16.0);

    QSignalSpy spy(&clock, &FrameClock::frameTimeExceeded);

    grid.setSurfaceSize(QSize(3840, 2160)); // 4K surface

    // Populate 64 tiles on a 8×8 grid
    const int cols = 8, rows = 8;
    const double tw = 3840.0 / cols;
    const double th = 2160.0 / rows;
    for (int r = 0; r < rows; ++r) {
        for (int c = 0; c < cols; ++c) {
            const uint32_t idx = static_cast<uint32_t>(r * cols + c);
            auto frame = dec.generateFrame(TileId(idx), 320, 180, 0);
            grid.setTile(TileId(idx), frame,
                         QRectF(c * tw, r * th, tw, th));
        }
    }
    QCOMPARE(grid.tileCount(), 64);

    // 600-frame render loop
    for (int f = 0; f < 600; ++f) {
        clock.beginFrame();

        dec.beginFrame();
        grid.forEach([&](const TileEntry& entry) {
            auto frame = dec.generateFrame(entry.id,
                                           NullDecoderBackend::kDefaultWidth,
                                           NullDecoderBackend::kDefaultHeight,
                                           static_cast<std::int64_t>(f) * 33333LL);
            dec.drawTile(entry.id, frame, entry.dst);
        });
        dec.endFrame();
        dec.submitPresent();

        qc.evaluate(grid);
        clock.endFrame();
    }

    QCOMPARE(clock.frameCount(), std::size_t(600));

    // On a software-only path (no GPU, no display) the loop should complete
    // well under the 16 ms P99 budget.
    const double p99 = clock.p99Ms();
    qInfo() << "600-frame P99 (software rasterizer):" << p99 << "ms";
    QVERIFY2(p99 < 16.0,
             qPrintable(QString("P99 %1 ms exceeded 16 ms budget").arg(p99)));

    // No budget violations should have been signalled
    QCOMPARE(spy.count(), 0);

    dec.shutdown();
}

// ---------------------------------------------------------------------------
QTEST_MAIN(tst_render_core)
#include "tst_render_core.moc"
