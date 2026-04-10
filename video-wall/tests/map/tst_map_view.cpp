#include <QtTest/QtTest>

#include <QColor>
#include <QHash>
#include <QImage>
#include <QJsonDocument>
#include <QPointF>
#include <QSignalSpy>
#include <QSize>
#include <QTemporaryDir>

#include "map/CameraPlacementModel.h"
#include "map/FilesystemTileProvider.h"
#include "map/HeatmapOverlay.h"
#include "map/MapTypes.h"
#include "map/MapView.h"
#include "map/NullTileProvider.h"

#include <memory>

using namespace Kaivue::Map;

class tst_map_view : public QObject {
    Q_OBJECT

private slots:
    // Model serialization + hit-testing
    void placement_upsert_and_contains();
    void placement_jsonRoundTrip();
    void placement_loadRejectsMalformed();
    void placement_hitTest_pickClosest();
    void placement_hitTest_missOutsideRadius();

    // Heatmap
    void heatmap_clampsOutOfRangeValues();
    void heatmap_paintsOnlyPinsWithIntensity();
    void heatmap_rendersToImageNonZeroAlpha();

    // MapView mode switching + click routing
    void view_defaultsToFloorPlan();
    void view_modeSwitchEmitsSignalOnce();
    void view_handlesClick_emitsCameraSelected();
    void view_handlesClick_missEmitsNothing();

    // Tile providers
    void nullTileProvider_recordsFetches();
    void filesystemTileProvider_returnsFalseForMissingTile();
    void filesystemTileProvider_rejectsOutOfRangeZoom();
};

// ---------------------------------------------------------------------------
// Placement model
// ---------------------------------------------------------------------------

void tst_map_view::placement_upsert_and_contains() {
    CameraPlacementModel m;
    CameraPlacement p;
    p.id       = CameraId(QStringLiteral("cam-1"));
    p.position = QPointF(10.0, 20.0);
    p.label    = QStringLiteral("Lobby");

    QSignalSpy added(&m, &CameraPlacementModel::placementAdded);
    m.upsert(p);
    QCOMPARE(added.count(), 1);
    QVERIFY(m.contains(CameraId(QStringLiteral("cam-1"))));
    QCOMPARE(m.count(), 1);

    // Upserting again replaces in place, does not re-add.
    QSignalSpy changed(&m, &CameraPlacementModel::placementChanged);
    p.label = QStringLiteral("Main Lobby");
    m.upsert(p);
    QCOMPARE(changed.count(), 1);
    QCOMPARE(m.count(), 1);
    QCOMPARE(m.find(p.id)->label, QStringLiteral("Main Lobby"));
}

void tst_map_view::placement_jsonRoundTrip() {
    CameraPlacementModel src;
    CameraPlacement a;
    a.id = CameraId(QStringLiteral("cam-a"));
    a.position = QPointF(-73.5, 40.7);
    a.label = QStringLiteral("Front door");
    a.rotation = 45.0;
    CameraPlacement b;
    b.id = CameraId(QStringLiteral("cam-b"));
    b.position = QPointF(-73.6, 40.8);
    b.rotation = 180.0;

    src.upsert(a);
    src.upsert(b);

    const QByteArray bytes = src.toJsonBytes();
    QVERIFY(!bytes.isEmpty());

    CameraPlacementModel dst;
    QVERIFY(dst.loadFromJsonBytes(bytes));
    QCOMPARE(dst.count(), 2);

    const CameraPlacement* copyA = dst.find(CameraId(QStringLiteral("cam-a")));
    QVERIFY(copyA != nullptr);
    QCOMPARE(copyA->position.x(), -73.5);
    QCOMPARE(copyA->position.y(),  40.7);
    QCOMPARE(copyA->label, QStringLiteral("Front door"));
    QCOMPARE(copyA->rotation, 45.0);

    const CameraPlacement* copyB = dst.find(CameraId(QStringLiteral("cam-b")));
    QVERIFY(copyB != nullptr);
    QCOMPARE(copyB->rotation, 180.0);
}

void tst_map_view::placement_loadRejectsMalformed() {
    CameraPlacementModel m;
    // Pre-populate to verify failure leaves state untouched.
    CameraPlacement p;
    p.id = CameraId(QStringLiteral("keep-me"));
    p.position = QPointF(1.0, 2.0);
    m.upsert(p);

    // Wrong version.
    QVERIFY(!m.loadFromJsonBytes(R"({"version":2,"placements":[]})"));
    // Placements not an array.
    QVERIFY(!m.loadFromJsonBytes(R"({"version":1,"placements":"nope"})"));
    // Placement entry missing id.
    QVERIFY(!m.loadFromJsonBytes(R"({"version":1,"placements":[{"x":1,"y":2}]})"));
    // Parse error.
    QVERIFY(!m.loadFromJsonBytes(QByteArray("not-json")));

    // State preserved on every failure.
    QCOMPARE(m.count(), 1);
    QVERIFY(m.contains(CameraId(QStringLiteral("keep-me"))));
}

void tst_map_view::placement_hitTest_pickClosest() {
    CameraPlacementModel m;
    CameraPlacement near;
    near.id = CameraId(QStringLiteral("near"));
    near.position = QPointF(100.0, 100.0);
    CameraPlacement far;
    far.id = CameraId(QStringLiteral("far"));
    far.position = QPointF(110.0, 100.0);

    m.upsert(far);    // inserted first
    m.upsert(near);   // inserted second

    // Click at (101, 100): near is 1 unit away, far is 9 units away.
    const CameraId hit = m.hitTest(QPointF(101.0, 100.0), /*radius=*/20.0);
    QCOMPARE(hit.value, QStringLiteral("near"));
}

void tst_map_view::placement_hitTest_missOutsideRadius() {
    CameraPlacementModel m;
    CameraPlacement p;
    p.id = CameraId(QStringLiteral("only"));
    p.position = QPointF(0.0, 0.0);
    m.upsert(p);

    const CameraId hit = m.hitTest(QPointF(100.0, 100.0), /*radius=*/5.0);
    QVERIFY(!hit.isValid());
}

// ---------------------------------------------------------------------------
// Heatmap
// ---------------------------------------------------------------------------

void tst_map_view::heatmap_clampsOutOfRangeValues() {
    HeatmapOverlay h;
    QHash<CameraId, float> values;
    values.insert(CameraId(QStringLiteral("lo")), -10.0f);
    values.insert(CameraId(QStringLiteral("hi")),  5.0f);
    h.setIntensities(values);

    QCOMPARE(h.intensityFor(CameraId(QStringLiteral("lo"))), 0.0f);
    QCOMPARE(h.intensityFor(CameraId(QStringLiteral("hi"))), 1.0f);
    QCOMPARE(h.intensityFor(CameraId(QStringLiteral("missing"))), 0.0f);
    QCOMPARE(h.sampleCount(), 2);
}

void tst_map_view::heatmap_paintsOnlyPinsWithIntensity() {
    CameraPlacementModel m;
    CameraPlacement a, b, c;
    a.id = CameraId(QStringLiteral("a"));
    a.position = QPointF(50.0, 50.0);
    b.id = CameraId(QStringLiteral("b"));
    b.position = QPointF(150.0, 150.0);
    c.id = CameraId(QStringLiteral("c"));
    c.position = QPointF(250.0, 250.0);
    m.upsert(a);
    m.upsert(b);
    m.upsert(c);

    HeatmapOverlay h;
    QHash<CameraId, float> values;
    values.insert(a.id, 0.9f);
    values.insert(b.id, 0.0f);  // zero -> should be skipped
    // c absent -> should be skipped
    h.setIntensities(values);

    QImage img(320, 320, QImage::Format_ARGB32_Premultiplied);
    img.fill(Qt::transparent);
    QPainter p(&img);
    const int drawn = h.paint(p, m);
    p.end();

    QCOMPARE(drawn, 1);
}

void tst_map_view::heatmap_rendersToImageNonZeroAlpha() {
    CameraPlacementModel m;
    CameraPlacement p;
    p.id = CameraId(QStringLiteral("solo"));
    p.position = QPointF(100.0, 100.0);
    m.upsert(p);

    HeatmapOverlay h;
    QHash<CameraId, float> values;
    values.insert(p.id, 1.0f);
    h.setIntensities(values);

    const QImage out = h.renderToImage(QSize(200, 200), m);
    QCOMPARE(out.size(), QSize(200, 200));

    // The circle center should be opaque-ish; a far corner should stay transparent.
    const QColor center = out.pixelColor(100, 100);
    const QColor corner = out.pixelColor(5, 5);
    QVERIFY(center.alpha() > 0);
    QCOMPARE(corner.alpha(), 0);
}

// ---------------------------------------------------------------------------
// MapView
// ---------------------------------------------------------------------------

void tst_map_view::view_defaultsToFloorPlan() {
    MapView v;
    QCOMPARE(v.mode(), MapMode::FloorPlan);
    QVERIFY(v.placements() != nullptr);
    QVERIFY(v.heatmap() != nullptr);
}

void tst_map_view::view_modeSwitchEmitsSignalOnce() {
    MapView v;
    QSignalSpy spy(&v, &MapView::modeChanged);

    v.setMode(MapMode::Geographic);
    QCOMPARE(spy.count(), 1);

    v.setMode(MapMode::Geographic); // idempotent
    QCOMPARE(spy.count(), 1);

    v.setMode(MapMode::FloorPlan);
    QCOMPARE(spy.count(), 2);
}

void tst_map_view::view_handlesClick_emitsCameraSelected() {
    MapView v;
    v.setHitRadius(10.0);

    CameraPlacement p;
    p.id = CameraId(QStringLiteral("clickme"));
    p.position = QPointF(200.0, 300.0);
    v.placements()->upsert(p);

    // Provide a NullTileProvider to prove wiring works headlessly.
    v.setTileProvider(std::make_shared<NullTileProvider>());

    QSignalSpy spy(&v, &MapView::cameraSelected);
    v.handleMapClick(QPointF(204.0, 301.0));
    QCOMPARE(spy.count(), 1);

    const QList<QVariant> args = spy.takeFirst();
    QCOMPARE(args.at(0).value<CameraId>().value, QStringLiteral("clickme"));
}

void tst_map_view::view_handlesClick_missEmitsNothing() {
    MapView v;
    v.setHitRadius(5.0);
    CameraPlacement p;
    p.id = CameraId(QStringLiteral("x"));
    p.position = QPointF(0.0, 0.0);
    v.placements()->upsert(p);

    QSignalSpy spy(&v, &MapView::cameraSelected);
    v.handleMapClick(QPointF(1000.0, 1000.0));
    QCOMPARE(spy.count(), 0);
}

// ---------------------------------------------------------------------------
// Tile providers
// ---------------------------------------------------------------------------

void tst_map_view::nullTileProvider_recordsFetches() {
    NullTileProvider np;
    QByteArray out;
    QVERIFY(!np.fetchTile(TileKey{3, 4, 5}, out));
    QVERIFY(!np.fetchTile(TileKey{6, 7, 8}, out));
    QCOMPARE(np.fetchedKeys().size(), 2);
    QCOMPARE(np.fetchedKeys()[0].z, 3);
    QCOMPARE(np.fetchedKeys()[1].x, 7);
    QCOMPARE(np.providerName(), QStringLiteral("null"));
}

void tst_map_view::filesystemTileProvider_returnsFalseForMissingTile() {
    QTemporaryDir dir;
    QVERIFY(dir.isValid());
    FilesystemTileProvider fp(dir.path(), 0, 19);
    QByteArray out;
    QVERIFY(!fp.fetchTile(TileKey{5, 1, 1}, out));
    QVERIFY(out.isEmpty());
    QVERIFY(fp.tilePath(TileKey{5, 1, 1}).endsWith(QStringLiteral("5/1/1.png")));
}

void tst_map_view::filesystemTileProvider_rejectsOutOfRangeZoom() {
    QTemporaryDir dir;
    QVERIFY(dir.isValid());
    FilesystemTileProvider fp(dir.path(), /*minZoom=*/5, /*maxZoom=*/10);
    QByteArray out;
    QVERIFY(!fp.fetchTile(TileKey{3, 0, 0}, out));  // below min
    QVERIFY(!fp.fetchTile(TileKey{12, 0, 0}, out)); // above max
    QVERIFY(!fp.fetchTile(TileKey{-1, 0, 0}, out)); // invalid key
}

QTEST_MAIN(tst_map_view)
#include "tst_map_view.moc"
