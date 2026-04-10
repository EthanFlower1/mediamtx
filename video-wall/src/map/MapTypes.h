#pragma once

//
// MapTypes.h — core value types for the video-wall map view (KAI-338).
//
// Defines strong-typed CameraId, world/scene coordinate placement, and
// map-mode enumeration. Kept header-only so both the runtime module and
// unit tests can include it without pulling Qt Quick at compile time.
//

#include <QHash>
#include <QMetaType>
#include <QPointF>
#include <QString>

#include <cstdint>
#include <functional>

namespace Kaivue::Map {

/**
 * Strong-typed camera identifier.
 *
 * Wraps a QString (stable across sessions, UUID or slug form) so that a
 * raw string cannot be accidentally passed where a CameraId is expected.
 */
struct CameraId {
    QString value;

    CameraId() = default;
    explicit CameraId(QString v) noexcept : value(std::move(v)) {}

    [[nodiscard]] bool isValid() const noexcept { return !value.isEmpty(); }

    bool operator==(const CameraId& other) const noexcept { return value == other.value; }
    bool operator!=(const CameraId& other) const noexcept { return value != other.value; }
    bool operator<(const CameraId& other)  const noexcept { return value <  other.value; }
};

/**
 * Map view mode — selects between geographic (Qt Location tiles) and
 * floor-plan (QGraphicsScene + raster background) backends.
 */
enum class MapMode {
    Geographic = 0,
    FloorPlan  = 1,
};

/**
 * A single camera pin placed on the map.
 *
 * For Geographic mode, `position` stores {longitude, latitude} in degrees.
 * For FloorPlan mode, `position` stores {x, y} scene coordinates (pixels).
 *
 * `label` is an optional operator-friendly display name; the video wall
 * still addresses the camera by CameraId.
 */
struct CameraPlacement {
    CameraId id;
    QPointF  position;     // {lon,lat} or {x,y} — depends on MapMode
    QString  label;        // optional; may be empty
    qreal    rotation{0};  // degrees, clockwise; used by directional icons
};

} // namespace Kaivue::Map

// ---------------------------------------------------------------------------
// Qt meta-type + QHash support
// ---------------------------------------------------------------------------
Q_DECLARE_METATYPE(Kaivue::Map::CameraId)
Q_DECLARE_METATYPE(Kaivue::Map::CameraPlacement)

namespace Kaivue::Map {

inline size_t qHash(const CameraId& id, size_t seed = 0) noexcept {
    return qHash(id.value, seed);
}

} // namespace Kaivue::Map

namespace std {
template <>
struct hash<Kaivue::Map::CameraId> {
    size_t operator()(const Kaivue::Map::CameraId& id) const noexcept {
        return std::hash<std::string>{}(id.value.toStdString());
    }
};
} // namespace std
