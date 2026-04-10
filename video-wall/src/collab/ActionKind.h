#pragma once

#include <QString>

namespace Kaivue::Collab {

/**
 * Enumerates operator-initiated actions that must be:
 *   1. Broadcast to other operators via ICollabTransport.
 *   2. Mediated by ConflictResolver (for mutually-exclusive actions).
 *   3. Written to IAuditSink for forensic review.
 */
enum class ActionKind {
    TileFocus,       // operator focused a camera tile
    PtzRequest,      // operator requested PTZ control of a camera
    LayoutChange,    // operator changed the wall layout / scene
    CursorMove,      // mouse cursor movement (not conflict-mediated)
    JoinSession,     // operator joined the wall
    LeaveSession,    // operator left the wall
};

[[nodiscard]] inline QString actionKindName(ActionKind k) {
    switch (k) {
        case ActionKind::TileFocus:    return QStringLiteral("TileFocus");
        case ActionKind::PtzRequest:   return QStringLiteral("PtzRequest");
        case ActionKind::LayoutChange: return QStringLiteral("LayoutChange");
        case ActionKind::CursorMove:   return QStringLiteral("CursorMove");
        case ActionKind::JoinSession:  return QStringLiteral("JoinSession");
        case ActionKind::LeaveSession: return QStringLiteral("LeaveSession");
    }
    return QStringLiteral("Unknown");
}

/**
 * True if this action kind is mutually-exclusive and therefore must go
 * through ConflictResolver (e.g. only one operator may hold PTZ on a
 * given camera at a time).
 */
[[nodiscard]] inline bool actionKindIsExclusive(ActionKind k) noexcept {
    switch (k) {
        case ActionKind::PtzRequest:
        case ActionKind::LayoutChange:
        case ActionKind::TileFocus:
            return true;
        default:
            return false;
    }
}

} // namespace Kaivue::Collab
