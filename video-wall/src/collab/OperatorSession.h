#pragma once

#include "OperatorId.h"

#include <QColor>
#include <QPointF>
#include <QString>

#include <chrono>

namespace Kaivue::Collab {

/**
 * Per-operator session metadata shown to other operators collaborating on
 * the same video wall.
 *
 * A session is created on join() and destroyed on leave(); cursor state
 * lives alongside (see CollaborationManager::CursorState) so the session
 * record itself stays tiny and copyable.
 */
struct OperatorSession {
    OperatorId id;
    QString    display_name;
    QColor     cursor_color;
};

/**
 * Live cursor state for one operator.
 *
 * Fields are plain data so CollaborationManager can snapshot them cheaply
 * for the render thread.
 */
struct CursorState {
    OperatorId id;
    QPointF    position{};          // wall-space coordinates
    int        monitor_index{-1};   // which physical monitor the cursor is on
    std::chrono::steady_clock::time_point last_update{};
};

} // namespace Kaivue::Collab
