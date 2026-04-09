#pragma once

#include "LayoutKind.h"
#include "MonitorId.h"

#include <cstdint>
#include <optional>
#include <vector>

namespace Kaivue::Layout {

/**
 * Tile slot identifier inside a single MonitorLayout.
 *
 * Tile indices are zero-based and bounded by tileCountFor(layoutKind).
 * They map onto Kaivue::Render::TileId at render-bind time (KAI-333).
 */
struct TileId {
    uint32_t value{0};

    constexpr TileId() noexcept = default;
    constexpr explicit TileId(uint32_t v) noexcept : value(v) {}

    constexpr bool operator==(const TileId& other) const noexcept { return value == other.value; }
    constexpr bool operator!=(const TileId& other) const noexcept { return value != other.value; }
    constexpr bool operator<(const TileId& other)  const noexcept { return value < other.value; }
};

/**
 * One monitor's layout assignment inside a Scene.
 *
 *   monitor      — physical display this layout applies to
 *   kind         — template (4x4, 6x6, etc.)
 *   cameras      — camera id assigned to each tile, indexed by tile slot
 *   focus_tile   — for Focus / PictureInPicture, identifies the enlarged tile
 */
struct MonitorLayout {
    MonitorId             monitor{};
    LayoutKind            kind{LayoutKind::Grid4x4};
    std::vector<CameraId> cameras{};
    std::optional<TileId> focus_tile{};

    bool operator==(const MonitorLayout& o) const {
        return monitor == o.monitor
            && kind == o.kind
            && cameras == o.cameras
            && focus_tile == o.focus_tile;
    }
};

} // namespace Kaivue::Layout
