#include "FakeClock.h"

#include <utility>

namespace Kaivue::Layout {

IClock::TimerId FakeClock::scheduleAfter(std::chrono::milliseconds delay,
                                         Callback callback)
{
    const TimerId id = m_nextId++;
    Entry e{id, m_now + delay, std::move(callback)};
    m_pending.emplace(e.fireAt, std::move(e));
    return id;
}

void FakeClock::cancel(TimerId id)
{
    for (auto it = m_pending.begin(); it != m_pending.end(); ++it) {
        if (it->second.id == id) {
            m_pending.erase(it);
            return;
        }
    }
}

void FakeClock::advanceBy(std::chrono::milliseconds delta)
{
    const auto target = m_now + delta;

    while (!m_pending.empty()) {
        auto it = m_pending.begin();
        if (it->first > target) {
            break;
        }
        // Pop before invoking — callback may schedule new timers.
        Entry e = std::move(it->second);
        m_pending.erase(it);
        m_now = e.fireAt;
        if (e.callback) {
            e.callback();
        }
    }

    m_now = target;
}

} // namespace Kaivue::Layout
