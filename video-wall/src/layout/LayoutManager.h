#pragma once

#include "EventTrigger.h"
#include "IClock.h"
#include "IDisplayBus.h"
#include "MonitorLayout.h"
#include "Salvo.h"
#include "Scene.h"
#include "Tour.h"

#include <QHash>
#include <QObject>

#include <optional>
#include <vector>

namespace Kaivue::Layout {

/**
 * LayoutManager — owns the workstation's current scene + per-monitor
 * layouts and exposes scene / salvo / tour / alarm operations.
 *
 * Threading: created on the GUI thread.  All public methods must be
 * called from that thread.  Tour callbacks fire on the same thread via
 * the injected IClock (RealClock = QTimer, FakeClock = manual advance).
 *
 * Pure data semantics: switchScene() mutates this object's view of the
 * world and emits signals.  Render-side tile rebinding is the
 * responsibility of KAI-333; the data mutation alone must complete in
 * <50 ms even for 32 monitors so the user-visible <500 ms budget has
 * headroom for the GPU pipeline.
 */
class LayoutManager : public QObject {
    Q_OBJECT
public:
    /**
     * @param displayBus  Source of monitor topology (real or mock).
     * @param clock       Clock for tour timing (real or fake).  May be
     *                    nullptr if the caller never uses tour mode.
     */
    LayoutManager(IDisplayBus* displayBus,
                  IClock* clock,
                  QObject* parent = nullptr);
    ~LayoutManager() override;

    // ---- Scene library ----
    void addScene(const Scene& scene);
    [[nodiscard]] bool hasScene(SceneId id) const;
    [[nodiscard]] const Scene* sceneById(SceneId id) const;
    [[nodiscard]] std::vector<SceneId> sceneIds() const;

    // ---- Salvo library ----
    void addSalvo(const Salvo& salvo);
    [[nodiscard]] bool hasSalvo(SalvoId id) const;

    // ---- Tour library ----
    void addTour(const Tour& tour);
    [[nodiscard]] bool hasTour(TourId id) const;

    // ---- Event trigger map ----
    void registerTrigger(const EventTrigger& trigger);
    void clearTriggers();

    // ---- Operations ----
    /**
     * Apply the named scene to all monitors.  Emits monitorLayoutChanged
     * for every monitor whose layout changed and sceneChanged once.
     * Returns true if the scene was found and applied.
     */
    bool switchScene(SceneId id);

    /**
     * Apply a salvo: replace camera assignments per monitor while
     * preserving the existing layout kind.  Emits monitorLayoutChanged
     * for each affected monitor.  Returns true on success.
     */
    bool runSalvo(SalvoId id);

    /**
     * Begin a tour.  Returns false if the tour id is unknown or has no
     * steps.  Stops any tour already in progress.
     */
    bool startTour(TourId id);

    /**
     * Cancel any active tour.
     */
    void stopTour();

    /**
     * Whether a tour is currently running.
     */
    [[nodiscard]] bool tourActive() const { return m_activeTour.has_value(); }

    /**
     * Dispatch an alarm: looks up the registered trigger and either
     * switches scene or runs salvo.
     */
    bool onAlarmEvent(AlarmId alarm);

    // ---- Inspection ----
    [[nodiscard]] std::optional<SceneId> currentSceneId() const { return m_currentScene; }
    [[nodiscard]] const MonitorLayout* currentLayout(MonitorId monitor) const;
    [[nodiscard]] std::vector<MonitorLayout> currentLayouts() const;

signals:
    void sceneChanged(Kaivue::Layout::SceneId id);
    void monitorLayoutChanged(Kaivue::Layout::MonitorId monitor,
                              Kaivue::Layout::MonitorLayout layout);
    void salvoFired(Kaivue::Layout::SalvoId id);
    void tourStarted(Kaivue::Layout::TourId id);
    void tourStopped(Kaivue::Layout::TourId id);
    void tourStepAdvanced(Kaivue::Layout::TourId id, int stepIndex);
    void alarmHandled(Kaivue::Layout::AlarmId alarm);

private:
    struct ActiveTour {
        TourId          id;
        size_t          stepIndex{0};
        IClock::TimerId timer{0};
    };

    void applyMonitorLayout(const MonitorLayout& layout);
    void scheduleNextTourStep();
    void advanceTourStep();

    IDisplayBus* m_displayBus{nullptr};
    IClock*      m_clock{nullptr};

    QHash<uint32_t, Scene>  m_scenes;   // keyed by SceneId.value
    QHash<uint32_t, Salvo>  m_salvos;
    QHash<uint32_t, Tour>   m_tours;
    QHash<uint32_t, EventAction> m_triggers; // keyed by AlarmId.value

    QHash<uint32_t, MonitorLayout> m_currentByMonitor; // keyed by MonitorId.value
    std::optional<SceneId>         m_currentScene;
    std::optional<ActiveTour>      m_activeTour;
};

} // namespace Kaivue::Layout
