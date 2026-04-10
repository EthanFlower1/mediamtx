#include "CursorOverlay.h"

#include "CollaborationManager.h"

#include <QSGNode>

namespace Kaivue::Collab {

CursorOverlay::CursorOverlay(QQuickItem* parent)
    : QQuickItem(parent) {
    setFlag(ItemHasContents, true);
}

CursorOverlay::~CursorOverlay() = default;

void CursorOverlay::setManager(CollaborationManager* manager) {
    if (m_manager == manager) return;
    m_manager = manager;
    refresh();
    update();
}

void CursorOverlay::setLocalOperator(OperatorId id) {
    if (m_localOperator == id) return;
    m_localOperator = id;
    emit localOperatorChanged();
    refresh();
    update();
}

void CursorOverlay::refresh() {
    if (!m_manager) {
        std::lock_guard<std::mutex> lock(m_markersMutex);
        m_markers.clear();
        return;
    }

    // Lock-free snapshot read.
    auto snap = m_manager->currentSnapshot();
    auto ops  = m_manager->operators();

    std::unordered_map<OperatorId, OperatorSession> byId;
    byId.reserve(ops.size());
    for (auto& s : ops) byId.emplace(s.id, std::move(s));

    std::vector<Marker> built;
    built.reserve(snap ? snap->cursors.size() : 0);
    if (snap) {
        for (const auto& c : snap->cursors) {
            if (c.id == m_localOperator) continue; // skip local
            auto it = byId.find(c.id);
            if (it == byId.end()) continue;
            built.push_back(Marker{
                c.id,
                c.position,
                it->second.cursor_color,
                it->second.display_name,
                c.monitor_index,
            });
        }
    }

    {
        std::lock_guard<std::mutex> lock(m_markersMutex);
        m_markers = std::move(built);
    }
}

std::vector<CursorOverlay::Marker> CursorOverlay::markers() const {
    std::lock_guard<std::mutex> lock(m_markersMutex);
    return m_markers;
}

QSGNode* CursorOverlay::updatePaintNode(QSGNode* oldNode, UpdatePaintNodeData*) {
    // Scaffold: no scene-graph geometry yet.  A future KAI-339 follow-up
    // will build QSGGeometryNode markers here using OperatorSession
    // cursor_color and display_name.  For Wave 1 the markers() accessor
    // is sufficient for headless tests to validate state.
    return oldNode;
}

} // namespace Kaivue::Collab
