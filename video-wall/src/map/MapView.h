#pragma once

//
// MapView.h — operator-facing map controller (KAI-338).
//
// MapView is the façade the rest of the video-wall talks to. It owns:
//
//   * a MapMode (Geographic or FloorPlan)
//   * a CameraPlacementModel (pin data, hit-testing, JSON persistence)
//   * an ITileProvider (Geographic mode only, injected for testability)
//   * a HeatmapOverlay (intensity map)
//   * a floor-plan background image (FloorPlan mode only)
//
// MapView does NOT own a QQuickItem or a QGraphicsView directly — those
// are UI shells that bind to this controller. That keeps the core
// testable headlessly and cleanly separates presentation from state.
//
// Click routing: mapClicked(point) is fed from either the Quick view
// (geographic) or the QGraphicsScene (floor plan). MapView hit-tests
// the placement model and emits cameraSelected(id) when a pin is hit.
// The preview panel itself is wired up in a separate ticket.
//

#include "map/CameraPlacementModel.h"
#include "map/HeatmapOverlay.h"
#include "map/ITileProvider.h"
#include "map/MapTypes.h"

#include <QImage>
#include <QObject>
#include <QPointF>
#include <QString>

#include <memory>

namespace Kaivue::Map {

class MapView : public QObject {
    Q_OBJECT
    Q_PROPERTY(Kaivue::Map::MapMode mode READ mode WRITE setMode NOTIFY modeChanged)

public:
    explicit MapView(QObject* parent = nullptr);
    ~MapView() override;

    // ------------------------------------------------------------------
    // Mode
    // ------------------------------------------------------------------
    [[nodiscard]] MapMode mode() const noexcept { return m_mode; }
    void setMode(MapMode newMode);

    // ------------------------------------------------------------------
    // Dependency injection
    // ------------------------------------------------------------------

    /**
     * Install the slippy tile source used by Geographic mode. Ownership
     * is shared — the MapView keeps a strong reference so tests can
     * swap in a NullTileProvider without worrying about lifetime.
     */
    void setTileProvider(std::shared_ptr<ITileProvider> provider);

    [[nodiscard]] std::shared_ptr<ITileProvider> tileProvider() const noexcept {
        return m_tileProvider;
    }

    /**
     * Install a floor-plan background image. Used by FloorPlan mode.
     * An empty QImage clears the current background.
     */
    void setFloorPlanImage(const QImage& image);

    [[nodiscard]] const QImage& floorPlanImage() const noexcept { return m_floorPlan; }

    // ------------------------------------------------------------------
    // Accessors for wired-up views / tests
    // ------------------------------------------------------------------
    [[nodiscard]] CameraPlacementModel* placements() noexcept { return &m_model; }
    [[nodiscard]] const CameraPlacementModel* placements() const noexcept { return &m_model; }

    [[nodiscard]] HeatmapOverlay*       heatmap()    noexcept { return &m_heatmap; }
    [[nodiscard]] const HeatmapOverlay* heatmap() const noexcept { return &m_heatmap; }

    /**
     * Hit-test radius used when routing click events. Expressed in the
     * active coordinate system (scene px for FloorPlan, "logical pixels
     * at current zoom" for Geographic — the Quick item converts before
     * forwarding). Defaults to 24 units.
     */
    void setHitRadius(qreal radius) noexcept { m_hitRadius = radius; }
    [[nodiscard]] qreal hitRadius() const noexcept { return m_hitRadius; }

public slots:
    /**
     * Forward a click from the active UI shell. The slot performs hit
     * testing against the placement model and emits cameraSelected()
     * if a pin was struck.
     */
    void handleMapClick(const QPointF& point);

    /**
     * Update the heatmap intensities (observable by the UI shell via
     * heatmapChanged()).
     */
    void setHeatmapIntensities(const QHash<CameraId, float>& values);

signals:
    void modeChanged(Kaivue::Map::MapMode newMode);
    void tileProviderChanged();
    void floorPlanImageChanged();
    void heatmapChanged();
    void cameraSelected(const Kaivue::Map::CameraId& id);

private:
    MapMode                         m_mode{MapMode::FloorPlan};
    CameraPlacementModel            m_model;
    HeatmapOverlay                  m_heatmap;
    std::shared_ptr<ITileProvider>  m_tileProvider;
    QImage                          m_floorPlan;
    qreal                           m_hitRadius{24.0};
};

} // namespace Kaivue::Map
