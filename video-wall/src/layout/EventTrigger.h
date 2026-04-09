#pragma once

#include "Salvo.h"
#include "Scene.h"

#include <cstdint>
#include <variant>

namespace Kaivue::Layout {

/**
 * Strong-typed Alarm identifier.
 *
 * Alarms originate from external systems (ONVIF events, NVR motion
 * detection, access control) and are routed through onAlarmEvent().
 */
struct AlarmId {
    uint32_t value{0};

    constexpr AlarmId() noexcept = default;
    constexpr explicit AlarmId(uint32_t v) noexcept : value(v) {}

    constexpr bool operator==(const AlarmId& other) const noexcept { return value == other.value; }
    constexpr bool operator!=(const AlarmId& other) const noexcept { return value != other.value; }
    constexpr bool operator<(const AlarmId& other)  const noexcept { return value < other.value; }
};

/**
 * Action to perform when an alarm fires.  Either switch to a saved
 * Scene or fire a Salvo.
 */
using EventAction = std::variant<SceneId, SalvoId>;

/**
 * Mapping from alarm to layout action.
 */
struct EventTrigger {
    AlarmId     alarm{};
    EventAction action{};
};

inline uint qHash(const AlarmId& id, uint seed = 0) noexcept {
    return ::qHash(id.value, seed);
}

} // namespace Kaivue::Layout
