#pragma once

#include "Scene.h"

#include <QString>

#include <cstdint>
#include <vector>

namespace Kaivue::Layout {

/**
 * Strong-typed Tour identifier.
 */
struct TourId {
    uint32_t value{0};

    constexpr TourId() noexcept = default;
    constexpr explicit TourId(uint32_t v) noexcept : value(v) {}

    constexpr bool operator==(const TourId& other) const noexcept { return value == other.value; }
    constexpr bool operator!=(const TourId& other) const noexcept { return value != other.value; }
    constexpr bool operator<(const TourId& other)  const noexcept { return value < other.value; }
};

/**
 * One step in a tour: which scene to show, and how long to dwell on it.
 */
struct TourStep {
    SceneId scene{};
    int     dwell_seconds{10};
};

/**
 * Tour: a scheduled cycle through a series of scenes.
 *
 * If `loop` is true the tour wraps around forever; otherwise the tour
 * stops after the last step's dwell expires.
 */
struct Tour {
    TourId                id{};
    QString               name;
    std::vector<TourStep> steps{};
    bool                  loop{true};
};

} // namespace Kaivue::Layout
