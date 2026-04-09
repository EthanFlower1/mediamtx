#pragma once

#include "MonitorId.h"

#include <QRect>

namespace Kaivue::Layout {

/**
 * Abstract bus exposing the set of physical monitors attached to the
 * workstation.
 *
 * Real implementation (DisplayBus) wraps QGuiApplication::screens() and
 * is the only place in the codebase allowed to call into Qt windowing.
 *
 * MockDisplayBus (test-only) returns a fixed N-monitor configuration so
 * the layout manager can be exercised in headless CI.
 */
class IDisplayBus {
public:
    virtual ~IDisplayBus() = default;

    /**
     * Number of monitors currently attached.  Range: 1..32 in production.
     */
    virtual uint32_t monitorCount() const = 0;

    /**
     * Geometry of the given monitor in virtual desktop coordinates.
     * Returns an empty QRect if the monitor id is out of range.
     */
    virtual QRect monitorRect(MonitorId id) const = 0;
};

/**
 * MockDisplayBus — fixed N-monitor configuration for tests.
 *
 * Lays out monitors as a horizontal strip, each 1920×1080.
 */
class MockDisplayBus : public IDisplayBus {
public:
    explicit MockDisplayBus(uint32_t count) : m_count(count) {}

    uint32_t monitorCount() const override { return m_count; }

    QRect monitorRect(MonitorId id) const override {
        if (id.value >= m_count) {
            return QRect{};
        }
        return QRect(static_cast<int>(id.value) * 1920, 0, 1920, 1080);
    }

private:
    uint32_t m_count;
};

} // namespace Kaivue::Layout
