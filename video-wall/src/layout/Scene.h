#pragma once

#include "MonitorLayout.h"

#include <QString>

#include <cstdint>
#include <vector>

namespace Kaivue::Layout {

/**
 * Strong-typed Scene identifier.
 */
struct SceneId {
    uint32_t value{0};

    constexpr SceneId() noexcept = default;
    constexpr explicit SceneId(uint32_t v) noexcept : value(v) {}

    constexpr bool operator==(const SceneId& other) const noexcept { return value == other.value; }
    constexpr bool operator!=(const SceneId& other) const noexcept { return value != other.value; }
    constexpr bool operator<(const SceneId& other)  const noexcept { return value < other.value; }
};

/**
 * A Scene is a complete, named "what every monitor shows" preset.
 *
 * Scenes are saved to JSON and loaded at startup; switchScene() applies
 * a scene to the LayoutManager and is the basis for tour mode.
 */
struct Scene {
    SceneId                    id{};
    QString                    name;
    std::vector<MonitorLayout> monitor_layouts{};
};

} // namespace Kaivue::Layout
