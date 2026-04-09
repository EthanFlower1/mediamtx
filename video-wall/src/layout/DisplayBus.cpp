#include "DisplayBus.h"

#include <QGuiApplication>
#include <QScreen>

namespace Kaivue::Layout {

uint32_t DisplayBus::monitorCount() const
{
    const auto screens = QGuiApplication::screens();
    return static_cast<uint32_t>(screens.size());
}

QRect DisplayBus::monitorRect(MonitorId id) const
{
    const auto screens = QGuiApplication::screens();
    if (id.value >= static_cast<uint32_t>(screens.size())) {
        return QRect{};
    }
    return screens.at(static_cast<int>(id.value))->geometry();
}

} // namespace Kaivue::Layout
