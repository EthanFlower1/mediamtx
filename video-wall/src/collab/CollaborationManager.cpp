#include "CollaborationManager.h"

#include "ConflictResolver.h"

#include <algorithm>
#include <utility>

namespace Kaivue::Collab {

CollaborationManager::CollaborationManager(ICollabTransport* transport,
                                           IAuditSink*       audit,
                                           QObject*          parent)
    : QObject(parent),
      m_transport(transport),
      m_audit(audit),
      m_resolver(std::make_unique<ConflictResolver>(this)),
      m_snapshot(std::make_shared<const CursorSnapshot>()) {

    if (m_transport) {
        m_transport->onCursor([this](const CursorMessage& msg) {
            onRemoteCursor(msg);
        });
        m_transport->onAction([this](const ActionMessage& msg) {
            onRemoteAction(msg);
        });
        m_transport->onSession([this](const SessionMessage& msg) {
            onRemoteSession(msg);
        });
    }
}

CollaborationManager::~CollaborationManager() = default;

void CollaborationManager::joinOperator(const OperatorSession& session) {
    {
        std::lock_guard<std::mutex> lock(m_stateMutex);
        m_sessions[session.id] = session;
        m_cursors[session.id]  = CursorState{
            session.id,
            QPointF{},
            -1,
            std::chrono::steady_clock::now(),
        };
        publishSnapshot();
    }

    auditAction(session.id, ActionKind::JoinSession, session.display_name);

    if (m_transport) {
        m_transport->sendSession(SessionMessage{session, /*joining=*/true});
    }
    emit operatorJoined(session.id);
}

void CollaborationManager::leaveOperator(OperatorId id) {
    OperatorSession sessionCopy;
    bool            had = false;
    {
        std::lock_guard<std::mutex> lock(m_stateMutex);
        auto it = m_sessions.find(id);
        if (it != m_sessions.end()) {
            sessionCopy = it->second;
            had = true;
            m_sessions.erase(it);
        }
        m_cursors.erase(id);
        publishSnapshot();
    }

    if (!had) return;

    auditAction(id, ActionKind::LeaveSession, sessionCopy.display_name);

    if (m_transport) {
        m_transport->sendSession(SessionMessage{sessionCopy, /*joining=*/false});
    }
    emit operatorLeft(id);
}

void CollaborationManager::updateLocalCursor(OperatorId id,
                                             QPointF    position,
                                             int        monitorIndex) {
    {
        std::lock_guard<std::mutex> lock(m_stateMutex);
        auto it = m_cursors.find(id);
        if (it == m_cursors.end()) return; // unknown operator
        it->second.position      = position;
        it->second.monitor_index = monitorIndex;
        it->second.last_update   = std::chrono::steady_clock::now();
        publishSnapshot();
    }

    if (m_auditCursor.load(std::memory_order_relaxed)) {
        auditAction(id, ActionKind::CursorMove, QString::number(monitorIndex));
    }

    if (m_transport) {
        m_transport->sendCursor(CursorMessage{id, position, monitorIndex});
    }
    emit cursorUpdated(id);
}

bool CollaborationManager::requestAction(OperatorId     id,
                                         ActionKind     kind,
                                         const QString& target) {
    if (actionKindIsExclusive(kind)) {
        if (!m_resolver->tryClaim(id, kind, target)) {
            return false;
        }
    }
    auditAction(id, kind, target);
    if (m_transport) {
        m_transport->sendAction(ActionMessage{id, kind, target});
    }
    emit actionCommitted(id, kind, target);
    return true;
}

void CollaborationManager::releaseAction(OperatorId     id,
                                         ActionKind     kind,
                                         const QString& target) {
    if (actionKindIsExclusive(kind)) {
        m_resolver->release(id, kind, target);
    }
}

std::size_t CollaborationManager::operatorCount() const {
    std::lock_guard<std::mutex> lock(m_stateMutex);
    return m_sessions.size();
}

std::vector<OperatorSession> CollaborationManager::operators() const {
    std::lock_guard<std::mutex> lock(m_stateMutex);
    std::vector<OperatorSession> out;
    out.reserve(m_sessions.size());
    for (const auto& [id, s] : m_sessions) out.push_back(s);
    return out;
}

std::shared_ptr<const CursorSnapshot> CollaborationManager::currentSnapshot() const {
    std::lock_guard<std::mutex> lock(m_snapshotMutex);
    return m_snapshot;
}

void CollaborationManager::publishSnapshot() {
    // Caller holds m_stateMutex.
    auto snap = std::make_shared<CursorSnapshot>();
    snap->cursors.reserve(m_cursors.size());
    for (const auto& [id, cs] : m_cursors) snap->cursors.push_back(cs);
    std::sort(snap->cursors.begin(), snap->cursors.end(),
              [](const CursorState& a, const CursorState& b) {
                  return a.id < b.id;
              });

    std::lock_guard<std::mutex> lock(m_snapshotMutex);
    m_snapshot = std::move(snap);
}

void CollaborationManager::auditAction(OperatorId     id,
                                       ActionKind     kind,
                                       const QString& target) {
    if (!m_audit) return;
    m_audit->log(AuditEntry{
        id,
        kind,
        target,
        std::chrono::system_clock::now(),
    });
}

// ---- Transport callbacks -------------------------------------------------

void CollaborationManager::onRemoteCursor(const CursorMessage& msg) {
    {
        std::lock_guard<std::mutex> lock(m_stateMutex);
        auto& cs = m_cursors[msg.id];
        cs.id             = msg.id;
        cs.position       = msg.position;
        cs.monitor_index  = msg.monitor_index;
        cs.last_update    = std::chrono::steady_clock::now();
        publishSnapshot();
    }
    emit cursorUpdated(msg.id);
}

void CollaborationManager::onRemoteAction(const ActionMessage& msg) {
    // Remote actions were already conflict-mediated by their originator
    // (in a centralised transport topology).  We still audit them locally
    // so every node has a complete trail.
    auditAction(msg.id, msg.kind, msg.target);
    emit actionCommitted(msg.id, msg.kind, msg.target);
}

void CollaborationManager::onRemoteSession(const SessionMessage& msg) {
    bool changed = false;
    {
        std::lock_guard<std::mutex> lock(m_stateMutex);
        if (msg.joining) {
            if (m_sessions.find(msg.session.id) == m_sessions.end()) {
                m_sessions[msg.session.id] = msg.session;
                m_cursors[msg.session.id]  = CursorState{
                    msg.session.id, QPointF{}, -1,
                    std::chrono::steady_clock::now()};
                changed = true;
            }
        } else {
            if (m_sessions.erase(msg.session.id) > 0) {
                m_cursors.erase(msg.session.id);
                changed = true;
            }
        }
        if (changed) publishSnapshot();
    }
    if (changed) {
        if (msg.joining) emit operatorJoined(msg.session.id);
        else             emit operatorLeft(msg.session.id);
    }
}

} // namespace Kaivue::Collab
