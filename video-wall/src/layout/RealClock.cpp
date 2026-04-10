#include "RealClock.h"

#include <QTimer>

#include <chrono>

namespace Kaivue::Layout {

RealClock::RealClock(QObject* parent)
    : QObject(parent)
{
}

RealClock::~RealClock() = default;

std::chrono::milliseconds RealClock::now() const
{
    using namespace std::chrono;
    return duration_cast<milliseconds>(steady_clock::now().time_since_epoch());
}

IClock::TimerId RealClock::scheduleAfter(std::chrono::milliseconds delay,
                                         Callback callback)
{
    const TimerId id = m_nextId++;
    auto* timer = new QTimer(this);
    timer->setSingleShot(true);
    timer->setInterval(static_cast<int>(delay.count()));

    QObject::connect(timer, &QTimer::timeout, this, [this, id, cb = std::move(callback)]() {
        // Remove from map first so re-entrant scheduleAfter() inside cb is fine.
        if (auto it = m_timers.find(id); it != m_timers.end()) {
            it.value()->deleteLater();
            m_timers.erase(it);
        }
        if (cb) cb();
    });

    m_timers.insert(id, timer);
    timer->start();
    return id;
}

void RealClock::cancel(TimerId id)
{
    auto it = m_timers.find(id);
    if (it == m_timers.end()) return;
    it.value()->stop();
    it.value()->deleteLater();
    m_timers.erase(it);
}

} // namespace Kaivue::Layout
