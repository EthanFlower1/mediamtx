#include "map/MapView.h"

namespace Kaivue::Map {

MapView::MapView(QObject* parent)
    : QObject(parent)
{
    // Register Qt meta-types once so queued signals and QSignalSpy can
    // marshal these values across thread boundaries.
    static const int camIdType    = qRegisterMetaType<CameraId>("Kaivue::Map::CameraId");
    static const int camPlaceType = qRegisterMetaType<CameraPlacement>("Kaivue::Map::CameraPlacement");
    static const int modeType     = qRegisterMetaType<MapMode>("Kaivue::Map::MapMode");
    (void)camIdType; (void)camPlaceType; (void)modeType;
}

MapView::~MapView() = default;

void MapView::setMode(MapMode newMode) {
    if (m_mode == newMode) {
        return;
    }
    m_mode = newMode;
    emit modeChanged(m_mode);
}

void MapView::setTileProvider(std::shared_ptr<ITileProvider> provider) {
    if (m_tileProvider == provider) {
        return;
    }
    m_tileProvider = std::move(provider);
    emit tileProviderChanged();
}

void MapView::setFloorPlanImage(const QImage& image) {
    m_floorPlan = image;
    emit floorPlanImageChanged();
}

void MapView::handleMapClick(const QPointF& point) {
    const CameraId hit = m_model.hitTest(point, m_hitRadius);
    if (hit.isValid()) {
        emit cameraSelected(hit);
    }
}

void MapView::setHeatmapIntensities(const QHash<CameraId, float>& values) {
    m_heatmap.setIntensities(values);
    emit heatmapChanged();
}

} // namespace Kaivue::Map
