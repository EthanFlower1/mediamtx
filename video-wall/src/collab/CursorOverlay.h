#pragma once

#include "OperatorId.h"
#include "OperatorSession.h"

#include <QColor>
#include <QPointF>
#include <QQuickItem>
#include <QString>

#include <memory>
#include <unordered_map>
#include <vector>

namespace Kaivue::Collab {

class CollaborationManager;
struct CursorSnapshot;

/**
 * CursorOverlay — QQuickItem that draws one marker per REMOTE operator.
 *
 * The local operator's cursor is rendered by the OS as usual; this
 * overlay is only for the *other* operators collaborating on the same
 * wall.
 *
 * Each marker = small filled circle in OperatorSession::cursor_color
 * with the display name rendered above it.
 *
 * The overlay pulls snapshots from CollaborationManager via the
 * lock-free currentSnapshot() reader, so no locks are held on the
 * render thread.
 */
class CursorOverlay : public QQuickItem {
    Q_OBJECT
    Q_PROPERTY(OperatorId localOperator READ localOperator WRITE setLocalOperator
                   NOTIFY localOperatorChanged)
public:
    explicit CursorOverlay(QQuickItem* parent = nullptr);
    ~CursorOverlay() override;

    /**
     * Wire the overlay to a manager.  The overlay does not take
     * ownership; the manager must outlive it.
     */
    void setManager(CollaborationManager* manager);

    [[nodiscard]] OperatorId localOperator() const { return m_localOperator; }
    void setLocalOperator(OperatorId id);

    /**
     * Poll the manager and rebuild the draw list.  Called on each
     * Qt render tick (or from a QTimer).  Invoking this without a
     * manager is a no-op.
     */
    void refresh();

    /**
     * Marker metadata produced for the scene graph.  Exposed for
     * tests and for a future QSGNode-based renderer.
     */
    struct Marker {
        OperatorId id;
        QPointF    position;
        QColor     color;
        QString    label;
        int        monitor_index{-1};
    };

    [[nodiscard]] std::vector<Marker> markers() const;

    // QQuickItem override — scaffold: returns nullptr.  A future
    // implementation will build a QSGNode tree here.
    QSGNode* updatePaintNode(QSGNode* oldNode, UpdatePaintNodeData*) override;

signals:
    void localOperatorChanged();

private:
    CollaborationManager* m_manager{nullptr};
    OperatorId            m_localOperator{};
    mutable std::mutex    m_markersMutex;
    std::vector<Marker>   m_markers;
};

} // namespace Kaivue::Collab
