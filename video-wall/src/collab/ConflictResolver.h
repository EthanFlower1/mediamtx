#pragma once

#include "ActionKind.h"
#include "OperatorId.h"

#include <QObject>

#include <chrono>
#include <mutex>
#include <string>
#include <unordered_map>

namespace Kaivue::Collab {

/**
 * ConflictResolver — last-writer-wins with a grace window.
 *
 * Rules:
 *   - The first operator to claim `target` holds it.
 *   - Any other operator whose claim arrives within `graceMs` of the
 *     current holder's last-update is a conflict and LOSES (the
 *     current holder is preserved).  This is "first-writer-wins within
 *     the grace window" — the 500ms anti-flap interval.
 *   - After the grace window expires, the new claimant takes over
 *     (strict last-writer-wins).
 *
 * A conflict emits conflictDetected(winner, loser, kind) — winner is
 * the operator whose claim was rejected's counter-party (i.e. the
 * current holder); loser is the operator whose claim was rejected.
 *
 * Thread-safe.
 */
class ConflictResolver : public QObject {
    Q_OBJECT
public:
    static constexpr std::chrono::milliseconds kDefaultGrace{500};

    explicit ConflictResolver(QObject* parent = nullptr,
                              std::chrono::milliseconds grace = kDefaultGrace);
    ~ConflictResolver() override = default;

    /**
     * Attempt to claim `target` for `kind` on behalf of `claimant`.
     *
     * @return true if the claim is granted, false if it was rejected
     *         as a conflict (in which case conflictDetected() was
     *         emitted).
     */
    bool tryClaim(OperatorId claimant, ActionKind kind, const QString& target);

    /**
     * Release any claim the operator holds.  No-op if none.
     */
    void release(OperatorId claimant, ActionKind kind, const QString& target);

    /**
     * Current holder for a (kind, target) pair, or an invalid id.
     */
    [[nodiscard]] OperatorId currentHolder(ActionKind kind, const QString& target) const;

    void setGraceWindow(std::chrono::milliseconds grace);

signals:
    void conflictDetected(Kaivue::Collab::OperatorId winner,
                          Kaivue::Collab::OperatorId loser,
                          Kaivue::Collab::ActionKind kind);

private:
    struct Claim {
        OperatorId holder;
        std::chrono::steady_clock::time_point acquired;
    };

    // Key = "kind:target" so PTZ on cam-1 and LayoutChange on cam-1
    // don't collide.
    static std::string makeKey(ActionKind kind, const QString& target);

    mutable std::mutex                     m_mutex;
    std::unordered_map<std::string, Claim> m_claims;
    std::chrono::milliseconds              m_grace;
};

} // namespace Kaivue::Collab
