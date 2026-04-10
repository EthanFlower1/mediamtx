#pragma once

#include "IDisplayBus.h"

namespace Kaivue::Layout {

/**
 * Real DisplayBus — wraps QGuiApplication::screens().
 *
 * This is the ONLY translation unit in the codebase that may call into
 * Qt windowing for the purpose of monitor enumeration.  All consumers
 * of monitor topology must depend on IDisplayBus, not Qt screens
 * directly, so headless CI tests can swap in MockDisplayBus.
 */
class DisplayBus : public IDisplayBus {
public:
    DisplayBus() = default;
    ~DisplayBus() override = default;

    uint32_t monitorCount() const override;
    QRect    monitorRect(MonitorId id) const override;
};

} // namespace Kaivue::Layout
