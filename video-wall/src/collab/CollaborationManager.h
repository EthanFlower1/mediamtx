#pragma once

#include "ActionKind.h"
#include "IAuditSink.h"
#include "ICollabTransport.h"
#include "OperatorId.h"
#include "OperatorSession.h"

#include <QObject>
#include <QPointF>
#include <QString>

#include <atomic>
#include <memory>
#include <mutex>
#include <unordered_map>
#include <vector>

namespace Kaivue::Collab {

class ConflictResolver;

/**
 * Immutable snapshot of all cursor state at a point in time.
 *
 * Published by CollaborationManager via publishSnapshot() and read by
 * the render thread through currentSnapshot().  Because it is held by
 * std::shared_ptr and swapped atomically, the render thread never
 * blocks on the collab worker thread.
 */
struct CursorSnapshot {
    std::vector<CursorState> cursors;
};

/**
 * CollaborationManager — authoritative state for every operator
 * connected to one video wall.
 *
 * Responsibilities:
 *   - Tracks OperatorSession records (join / leave).
 *   - Maintains per-operator cursor state.
 *   - Publishes a lock-free, double-buffered CursorSnapshot so the
 *     render thread can draw remote cursors without blocking.
 *   - Routes mutually-exclusive actions (PTZ, layout, focus) through a
 *     ConflictResolver.
 *   - Writes every action to an IAuditSink.
 *   - Sends/receives via an ICollabTransport (LoopbackTransport for
 *     tests; WebSocket/WebRTC transport in production).
 *
 * Threading:
 *   - Public API is thread-safe.
 *   - Snapshot publish is atomic pointer swap; reader (render thread)
 *     calls currentSnapshot() with no locks.
 */
class CollaborationManager : public QObject {
    Q_OBJECT
public:
    CollaborationManager(ICollabTransport* transport,
                         IAuditSink*       audit,
                         QObject*          parent = nullptr);
    ~CollaborationManager() override;

    CollaborationManager(const CollaborationManager&)            = delete;
    CollaborationManager& operator=(const CollaborationManager&) = delete;

    /**
     * Register a local operator on this wall.  Broadcasts a session
     * join message and audits it.
     */
    void joinOperator(const OperatorSession& session);

    /**
     * Remove an operator.  Broadcasts a session leave message.
     */
    void leaveOperator(OperatorId id);

    /**
     * Report a local cursor move.  Updates local state, broadcasts via
     * transport, and republishes the snapshot.
     *
     * This is the hot path: called on every mouse move.  Intentionally
     * cheap — no audit write (high-frequency, low-value) unless you
     * opt in with auditCursor(true).
     */
    void updateLocalCursor(OperatorId id, QPointF position, int monitorIndex);

    /**
     * Request an exclusive action.  Routes through ConflictResolver;
     * if granted, broadcasts + audits.  Returns true on grant.
     */
    bool requestAction(OperatorId id, ActionKind kind, const QString& target);

    /**
     * Release an exclusive claim (operator moved on, PTZ done, etc.).
     */
    void releaseAction(OperatorId id, ActionKind kind, const QString& target);

    /**
     * Number of operators currently on this wall.
     */
    [[nodiscard]] std::size_t operatorCount() const;

    /**
     * Copy out the session records (small, infrequent).
     */
    [[nodiscard]] std::vector<OperatorSession> operators() const;

    /**
     * Lock-free snapshot read — safe from the render thread.
     */
    [[nodiscard]] std::shared_ptr<const CursorSnapshot> currentSnapshot() const;

    /**
     * Access the underlying resolver (tests + UI hook up the
     * conflictDetected signal).
     */
    [[nodiscard]] ConflictResolver* conflictResolver() const { return m_resolver.get(); }

    /**
     * When true, CursorMove actions are written to the audit sink as
     * well.  Off by default — high frequency, low forensic value.
     */
    void setAuditCursorMoves(bool on) { m_auditCursor = on; }

signals:
    void operatorJoined(Kaivue::Collab::OperatorId id);
    void operatorLeft(Kaivue::Collab::OperatorId id);
    void cursorUpdated(Kaivue::Collab::OperatorId id);
    void actionCommitted(Kaivue::Collab::OperatorId id,
                         Kaivue::Collab::ActionKind kind,
                         QString target);

private:
    // Transport callbacks — run on whatever thread the transport delivers on.
    void onRemoteCursor(const CursorMessage& msg);
    void onRemoteAction(const ActionMessage& msg);
    void onRemoteSession(const SessionMessage& msg);

    void publishSnapshot();          // Must be called with m_stateMutex held.
    void auditAction(OperatorId id, ActionKind kind, const QString& target);

    ICollabTransport* m_transport{nullptr};
    IAuditSink*       m_audit{nullptr};

    std::unique_ptr<ConflictResolver> m_resolver;

    mutable std::mutex                                m_stateMutex;
    std::unordered_map<OperatorId, OperatorSession>   m_sessions;
    std::unordered_map<OperatorId, CursorState>       m_cursors;

    // Atomic double-buffered snapshot for the render thread.
    mutable std::mutex                       m_snapshotMutex;
    std::shared_ptr<const CursorSnapshot>    m_snapshot;

    std::atomic<bool> m_auditCursor{false};
};

} // namespace Kaivue::Collab
