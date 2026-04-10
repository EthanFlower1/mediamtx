#pragma once

#include "IClock.h"

#include <QHash>
#include <QObject>

class QTimer;

namespace Kaivue::Layout {

/**
 * Real-time clock backed by QTimer single-shots and QElapsedTimer-style
 * monotonic now().
 *
 * Created on the LayoutManager's owning thread; QTimer affinity follows.
 */
class RealClock : public QObject, public IClock {
    Q_OBJECT
public:
    explicit RealClock(QObject* parent = nullptr);
    ~RealClock() override;

    std::chrono::milliseconds now() const override;

    TimerId scheduleAfter(std::chrono::milliseconds delay,
                          Callback callback) override;

    void cancel(TimerId id) override;

private:
    TimerId                  m_nextId{1};
    QHash<TimerId, QTimer*>  m_timers;
};

} // namespace Kaivue::Layout
