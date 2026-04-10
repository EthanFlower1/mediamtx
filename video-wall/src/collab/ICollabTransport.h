#pragma once

#include "ActionKind.h"
#include "OperatorId.h"
#include "OperatorSession.h"

#include <QPointF>
#include <QString>

#include <functional>
#include <mutex>
#include <vector>

namespace Kaivue::Collab {

/**
 * Cursor movement broadcast payload.
 */
struct CursorMessage {
    OperatorId id;
    QPointF    position;
    int        monitor_index{-1};
};

/**
 * Action broadcast payload (tile focus, PTZ, layout).
 */
struct ActionMessage {
    OperatorId id;
    ActionKind kind{ActionKind::CursorMove};
    QString    target;
};

/**
 * Session join/leave broadcast payload.
 */
struct SessionMessage {
    OperatorSession session;
    bool            joining{true};
};

/**
 * Transport abstraction — decouples CollaborationManager from the
 * network layer (Kaivue signalling / WebSocket / future mesh).
 *
 * A concrete transport must deliver messages to every subscriber
 * registered via onXxx() hooks, INCLUDING the sender (so the local
 * CollaborationManager sees its own echoes and can reconcile state).
 *
 * Implementations must be thread-safe.
 */
class ICollabTransport {
public:
    using CursorHandler  = std::function<void(const CursorMessage&)>;
    using ActionHandler  = std::function<void(const ActionMessage&)>;
    using SessionHandler = std::function<void(const SessionMessage&)>;

    virtual ~ICollabTransport() = default;

    virtual void sendCursor(const CursorMessage& msg)   = 0;
    virtual void sendAction(const ActionMessage& msg)   = 0;
    virtual void sendSession(const SessionMessage& msg) = 0;

    virtual void onCursor(CursorHandler handler)   = 0;
    virtual void onAction(ActionHandler handler)   = 0;
    virtual void onSession(SessionHandler handler) = 0;
};

/**
 * Purely in-process transport used by unit tests and the CI headless
 * path.  Fans every send out to every registered handler synchronously.
 * No network, no threads of its own.
 */
class LoopbackTransport final : public ICollabTransport {
public:
    void sendCursor(const CursorMessage& msg) override {
        std::vector<CursorHandler> handlers;
        {
            std::lock_guard<std::mutex> lock(m_mutex);
            handlers = m_cursorHandlers;
        }
        for (const auto& h : handlers) h(msg);
    }

    void sendAction(const ActionMessage& msg) override {
        std::vector<ActionHandler> handlers;
        {
            std::lock_guard<std::mutex> lock(m_mutex);
            handlers = m_actionHandlers;
        }
        for (const auto& h : handlers) h(msg);
    }

    void sendSession(const SessionMessage& msg) override {
        std::vector<SessionHandler> handlers;
        {
            std::lock_guard<std::mutex> lock(m_mutex);
            handlers = m_sessionHandlers;
        }
        for (const auto& h : handlers) h(msg);
    }

    void onCursor(CursorHandler handler) override {
        std::lock_guard<std::mutex> lock(m_mutex);
        m_cursorHandlers.push_back(std::move(handler));
    }

    void onAction(ActionHandler handler) override {
        std::lock_guard<std::mutex> lock(m_mutex);
        m_actionHandlers.push_back(std::move(handler));
    }

    void onSession(SessionHandler handler) override {
        std::lock_guard<std::mutex> lock(m_mutex);
        m_sessionHandlers.push_back(std::move(handler));
    }

private:
    mutable std::mutex          m_mutex;
    std::vector<CursorHandler>  m_cursorHandlers;
    std::vector<ActionHandler>  m_actionHandlers;
    std::vector<SessionHandler> m_sessionHandlers;
};

} // namespace Kaivue::Collab
