#pragma once

#include <QHash>

#include <cstdint>

namespace Kaivue::Layout {

/**
 * Strong-typed monitor identifier.  Wraps uint32_t to prevent implicit
 * conversion from raw integers (mirrors Kaivue::Render::TileId).
 *
 * Monitor IDs are dense and zero-based: a workstation driving 8 displays
 * uses MonitorId{0}..MonitorId{7}.
 */
struct MonitorId {
    uint32_t value{0};

    constexpr MonitorId() noexcept = default;
    constexpr explicit MonitorId(uint32_t v) noexcept : value(v) {}

    constexpr bool operator==(const MonitorId& other) const noexcept { return value == other.value; }
    constexpr bool operator!=(const MonitorId& other) const noexcept { return value != other.value; }
    constexpr bool operator<(const MonitorId& other)  const noexcept { return value < other.value; }
};

/**
 * Strong-typed camera identifier.  Cameras come from the NVR backend
 * and are referenced by uint32 row id from the cameras table.
 */
struct CameraId {
    uint32_t value{0};

    constexpr CameraId() noexcept = default;
    constexpr explicit CameraId(uint32_t v) noexcept : value(v) {}

    constexpr bool operator==(const CameraId& other) const noexcept { return value == other.value; }
    constexpr bool operator!=(const CameraId& other) const noexcept { return value != other.value; }
    constexpr bool operator<(const CameraId& other)  const noexcept { return value < other.value; }
};

inline uint qHash(const MonitorId& id, uint seed = 0) noexcept {
    return ::qHash(id.value, seed);
}

inline uint qHash(const CameraId& id, uint seed = 0) noexcept {
    return ::qHash(id.value, seed);
}

} // namespace Kaivue::Layout
