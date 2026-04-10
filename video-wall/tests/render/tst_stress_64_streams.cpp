#include <QtTest/QtTest>

#include <chrono>
#include <cstdint>

#include "render/DecodedFrame.h"
#include "render/FrameClock.h"
#include "render/NullDecoderBackend.h"
#include "render/QualityController.h"
#include "render/TileGrid.h"
#include "render/metrics/P99LatencyProbe.h"

using namespace Kaivue::Render;
using namespace Kaivue::Render::Metrics;

// ---------------------------------------------------------------------------
// 64-stream headless stress harness (KAI-333).
//
// Goal: drive the scaffold render pipeline with 64 concurrent "streams"
// (NullDecoderBackend tiles on an 8x8 grid) and assert that the P99
// decode-to-present latency stays under a documented budget.
//
// Budget: 33 ms (30 fps frame interval).
//   Rationale: The Kaivue Video Wall targets 30 fps minimum per tile for
//   64-tile walls. A single-frame-interval (33.33 ms) P99 budget means no
//   tile drops more than 1% of frames under nominal load. This is the
//   scaffold budget used by CI; the shipping budget for real HW decode
//   backends will be tightened in a follow-up ticket once libva / VideoToolbox
//   / D3D11VA paths are tuned.
//
// Note: the Null backend does not perform real GPU work. This harness
// validates the pipeline plumbing (TileGrid iteration, decode-call cadence,
// P99LatencyProbe wiring) rather than true GPU times. A follow-up under
// KAI-333 will swap the Null backend for a real hwaccel path and re-run
// this harness on target hardware.
// ---------------------------------------------------------------------------

namespace {
constexpr int kTileCols       = 8;
constexpr int kTileRows       = 8;
constexpr int kNumTiles       = kTileCols * kTileRows; // 64
constexpr int kFramesPerTile  = 60;   // ~2 seconds @ 30 fps (per tile)
constexpr int kBudgetUs       = 33000; // 33 ms — documented scaffold budget
} // namespace

class tst_stress_64_streams : public QObject {
    Q_OBJECT

private slots:
    void stress_64_streams_p99_under_33ms();
};

void tst_stress_64_streams::stress_64_streams_p99_under_33ms()
{
    NullDecoderBackend dec;
    QVERIFY(dec.init(nullptr));

    TileGrid          grid;
    QualityController qc;
    FrameClock        clock(33.0);
    P99LatencyProbe   probe(kNumTiles * kFramesPerTile);

    grid.setSurfaceSize(QSize(3840, 2160)); // 4K video wall

    const double tw = 3840.0 / kTileCols;
    const double th = 2160.0 / kTileRows;
    for (int r = 0; r < kTileRows; ++r) {
        for (int c = 0; c < kTileCols; ++c) {
            const auto idx = static_cast<uint32_t>(r * kTileCols + c);
            auto frame = dec.generateFrame(TileId(idx), 320, 180, 0);
            grid.setTile(TileId(idx), frame, QRectF(c * tw, r * th, tw, th));
        }
    }
    QCOMPARE(grid.tileCount(), kNumTiles);

    using Clock = std::chrono::steady_clock;

    for (int f = 0; f < kFramesPerTile; ++f) {
        clock.beginFrame();
        dec.beginFrame();

        grid.forEach([&](const TileEntry& entry) {
            // Simulate per-tile decode-to-present: timestamp around a
            // generateFrame() call and record the elapsed delta.
            const auto t0 = Clock::now();
            auto frame = dec.generateFrame(entry.id,
                                           NullDecoderBackend::kDefaultWidth,
                                           NullDecoderBackend::kDefaultHeight,
                                           static_cast<std::int64_t>(f) * 33333LL);
            dec.drawTile(entry.id, frame, entry.dst);
            const auto t1 = Clock::now();

            const auto latencyUs =
                static_cast<std::uint64_t>(
                    std::chrono::duration_cast<std::chrono::microseconds>(t1 - t0).count());
            probe.record(latencyUs);
        });

        dec.endFrame();
        dec.submitPresent();
        qc.evaluate(grid);
        clock.endFrame();
    }

    QCOMPARE(clock.frameCount(), std::size_t(kFramesPerTile));
    QCOMPARE(probe.size(), std::size_t(kNumTiles * kFramesPerTile));

    const auto p50 = probe.percentile(50.0);
    const auto p99 = probe.percentile(99.0);
    qInfo() << "64-stream stress harness — P50" << p50 << "us  P99" << p99
            << "us  (budget" << kBudgetUs << "us)";

    // Scaffold assertion: P99 decode-to-present latency stays under the
    // documented 33 ms budget.
    QVERIFY2(p99 < static_cast<std::uint64_t>(kBudgetUs),
             qPrintable(QStringLiteral("P99 %1 us exceeded %2 us budget")
                            .arg(p99).arg(kBudgetUs)));

    dec.shutdown();
}

QTEST_APPLESS_MAIN(tst_stress_64_streams)
#include "tst_stress_64_streams.moc"
