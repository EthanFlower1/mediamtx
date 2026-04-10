#pragma once

#include "IClock.h"

#include <chrono>
#include <map>
#include <vector>

namespace Kaivue::Layout {

/**
 * Deterministic test clock.
 *
 * Time only advances when advanceBy() is called; scheduled callbacks
 * fire in time order during the advance.  Re-entrant scheduling from
 * within a callback is supported (the new timer joins the queue and is
 * checked again before advanceBy() returns).
 */
class FakeClock : public IClock {
public:
    FakeClock() = default;
    ~FakeClock() override = default;

    std::chrono::milliseconds now() const override { return m_now; }

    TimerId scheduleAfter(std::chrono::milliseconds delay,
                          Callback callback) override;

    void cancel(TimerId id) override;

    /**
     * Advance virtual time by `delta`, firing every callback whose
     * scheduled time falls within the new window in chronological order.
     */
    void advanceBy(std::chrono::milliseconds delta);

    /**
     * Number of currently scheduled (un-fired, un-cancelled) timers.
     */
    [[nodiscard]] size_t pendingCount() const { return m_pending.size(); }

private:
    struct Entry {
        TimerId                    id;
        std::chrono::milliseconds  fireAt;
        Callback                   callback;
    };

    std::chrono::milliseconds m_now{0};
    TimerId                   m_nextId{1};
    // multimap keyed by fireAt — preserves chronological ordering even with
    // duplicates and supports stable iteration during advance.
    std::multimap<std::chrono::milliseconds, Entry> m_pending;
};

} // namespace Kaivue::Layout
