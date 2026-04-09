#include "LayoutManager.h"

#include <chrono>
#include <utility>

namespace Kaivue::Layout {

LayoutManager::LayoutManager(IDisplayBus* displayBus,
                             IClock* clock,
                             QObject* parent)
    : QObject(parent)
    , m_displayBus(displayBus)
    , m_clock(clock)
{
}

LayoutManager::~LayoutManager()
{
    stopTour();
}

// ---------------------------------------------------------------------------
// Library management
// ---------------------------------------------------------------------------
void LayoutManager::addScene(const Scene& scene)
{
    m_scenes.insert(scene.id.value, scene);
}

bool LayoutManager::hasScene(SceneId id) const
{
    return m_scenes.contains(id.value);
}

const Scene* LayoutManager::sceneById(SceneId id) const
{
    auto it = m_scenes.constFind(id.value);
    return it == m_scenes.constEnd() ? nullptr : &it.value();
}

std::vector<SceneId> LayoutManager::sceneIds() const
{
    std::vector<SceneId> out;
    out.reserve(static_cast<size_t>(m_scenes.size()));
    for (auto it = m_scenes.constBegin(); it != m_scenes.constEnd(); ++it) {
        out.push_back(SceneId(it.key()));
    }
    return out;
}

void LayoutManager::addSalvo(const Salvo& salvo)
{
    m_salvos.insert(salvo.id.value, salvo);
}

bool LayoutManager::hasSalvo(SalvoId id) const
{
    return m_salvos.contains(id.value);
}

void LayoutManager::addTour(const Tour& tour)
{
    m_tours.insert(tour.id.value, tour);
}

bool LayoutManager::hasTour(TourId id) const
{
    return m_tours.contains(id.value);
}

void LayoutManager::registerTrigger(const EventTrigger& trigger)
{
    m_triggers.insert(trigger.alarm.value, trigger.action);
}

void LayoutManager::clearTriggers()
{
    m_triggers.clear();
}

// ---------------------------------------------------------------------------
// Scene switching
// ---------------------------------------------------------------------------
void LayoutManager::applyMonitorLayout(const MonitorLayout& layout)
{
    auto it = m_currentByMonitor.find(layout.monitor.value);
    const bool changed = (it == m_currentByMonitor.end()) || !(it.value() == layout);
    m_currentByMonitor.insert(layout.monitor.value, layout);
    if (changed) {
        emit monitorLayoutChanged(layout.monitor, layout);
    }
}

bool LayoutManager::switchScene(SceneId id)
{
    auto it = m_scenes.constFind(id.value);
    if (it == m_scenes.constEnd()) {
        return false;
    }
    const Scene& scene = it.value();
    for (const auto& ml : scene.monitor_layouts) {
        applyMonitorLayout(ml);
    }
    m_currentScene = id;
    emit sceneChanged(id);
    return true;
}

bool LayoutManager::runSalvo(SalvoId id)
{
    auto it = m_salvos.constFind(id.value);
    if (it == m_salvos.constEnd()) {
        return false;
    }
    const Salvo& salvo = it.value();

    for (auto cit = salvo.cameras_per_monitor.constBegin();
         cit != salvo.cameras_per_monitor.constEnd(); ++cit) {
        const MonitorId mon = cit.key();
        MonitorLayout merged;
        if (auto cur = m_currentByMonitor.constFind(mon.value);
            cur != m_currentByMonitor.constEnd()) {
            merged = cur.value();
        } else {
            merged.monitor = mon;
            merged.kind = LayoutKind::Grid4x4;
        }
        merged.cameras = cit.value();
        applyMonitorLayout(merged);
    }
    emit salvoFired(id);
    return true;
}

// ---------------------------------------------------------------------------
// Tour mode
// ---------------------------------------------------------------------------
bool LayoutManager::startTour(TourId id)
{
    auto it = m_tours.constFind(id.value);
    if (it == m_tours.constEnd() || it.value().steps.empty()) {
        return false;
    }
    if (!m_clock) {
        return false;
    }
    stopTour();

    m_activeTour = ActiveTour{id, 0, 0};

    // Apply first step immediately, then schedule next.
    const Tour& tour = it.value();
    switchScene(tour.steps.front().scene);
    emit tourStarted(id);
    emit tourStepAdvanced(id, 0);
    scheduleNextTourStep();
    return true;
}

void LayoutManager::stopTour()
{
    if (!m_activeTour) return;
    if (m_clock && m_activeTour->timer != 0) {
        m_clock->cancel(m_activeTour->timer);
    }
    const TourId id = m_activeTour->id;
    m_activeTour.reset();
    emit tourStopped(id);
}

void LayoutManager::scheduleNextTourStep()
{
    if (!m_activeTour || !m_clock) return;
    auto it = m_tours.constFind(m_activeTour->id.value);
    if (it == m_tours.constEnd()) {
        m_activeTour.reset();
        return;
    }
    const Tour& tour = it.value();
    const TourStep& current = tour.steps[m_activeTour->stepIndex];
    const auto delay = std::chrono::seconds(current.dwell_seconds);

    m_activeTour->timer = m_clock->scheduleAfter(
        std::chrono::duration_cast<std::chrono::milliseconds>(delay),
        [this]() { advanceTourStep(); });
}

void LayoutManager::advanceTourStep()
{
    if (!m_activeTour) return;
    auto it = m_tours.constFind(m_activeTour->id.value);
    if (it == m_tours.constEnd()) {
        m_activeTour.reset();
        return;
    }
    const Tour& tour = it.value();
    const size_t next = m_activeTour->stepIndex + 1;

    if (next >= tour.steps.size()) {
        if (!tour.loop) {
            const TourId tid = m_activeTour->id;
            m_activeTour.reset();
            emit tourStopped(tid);
            return;
        }
        m_activeTour->stepIndex = 0;
    } else {
        m_activeTour->stepIndex = next;
    }

    const TourStep& step = tour.steps[m_activeTour->stepIndex];
    switchScene(step.scene);
    emit tourStepAdvanced(m_activeTour->id, static_cast<int>(m_activeTour->stepIndex));
    scheduleNextTourStep();
}

// ---------------------------------------------------------------------------
// Alarm dispatch
// ---------------------------------------------------------------------------
bool LayoutManager::onAlarmEvent(AlarmId alarm)
{
    auto it = m_triggers.constFind(alarm.value);
    if (it == m_triggers.constEnd()) {
        return false;
    }
    const EventAction& action = it.value();
    bool ok = false;
    if (std::holds_alternative<SceneId>(action)) {
        ok = switchScene(std::get<SceneId>(action));
    } else if (std::holds_alternative<SalvoId>(action)) {
        ok = runSalvo(std::get<SalvoId>(action));
    }
    if (ok) {
        emit alarmHandled(alarm);
    }
    return ok;
}

// ---------------------------------------------------------------------------
// Inspection
// ---------------------------------------------------------------------------
const MonitorLayout* LayoutManager::currentLayout(MonitorId monitor) const
{
    auto it = m_currentByMonitor.constFind(monitor.value);
    return it == m_currentByMonitor.constEnd() ? nullptr : &it.value();
}

std::vector<MonitorLayout> LayoutManager::currentLayouts() const
{
    std::vector<MonitorLayout> out;
    out.reserve(static_cast<size_t>(m_currentByMonitor.size()));
    for (auto it = m_currentByMonitor.constBegin();
         it != m_currentByMonitor.constEnd(); ++it) {
        out.push_back(it.value());
    }
    return out;
}

} // namespace Kaivue::Layout
