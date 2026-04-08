#include "TileGrid.h"

#include <algorithm>
#include <cassert>

namespace Kaivue::Render {

void TileGrid::setSurfaceSize(const QSize& size)
{
    m_surfaceSize = size.isValid() ? size : QSize{1920, 1080};
    // Recompute coverage for all existing tiles
    for (auto& [key, entry] : m_tiles) {
        recomputeCoverage(entry);
    }
}

void TileGrid::setTile(TileId id, const DecodedFrameRef& frame, const QRectF& dst)
{
    if (id.value >= static_cast<uint32_t>(kMaxTiles)) {
        // Defensive: production code should never exceed kMaxTiles
        return;
    }

    auto it = m_tiles.find(id.value);
    if (it == m_tiles.end()) {
        // New tile — record insertion order
        TileEntry entry{id, frame, dst, 0.0};
        recomputeCoverage(entry);
        m_tiles.emplace(id.value, entry);
        m_insertionOrder.push_back(id.value);
    } else {
        // Update existing tile
        it->second.frame = frame;
        it->second.dst   = dst;
        recomputeCoverage(it->second);
    }
}

void TileGrid::removeTile(TileId id)
{
    auto it = m_tiles.find(id.value);
    if (it == m_tiles.end()) return;

    m_tiles.erase(it);

    // Remove from insertion-order list
    auto oit = std::find(m_insertionOrder.begin(), m_insertionOrder.end(), id.value);
    if (oit != m_insertionOrder.end()) {
        m_insertionOrder.erase(oit);
    }
}

int TileGrid::tileCount() const noexcept
{
    return static_cast<int>(m_tiles.size());
}

void TileGrid::forEach(const std::function<void(const TileEntry&)>& visitor) const
{
    // Take a snapshot of the current insertion order so removeTile() during
    // the visitor does not invalidate our iteration.
    const std::vector<uint32_t> snapshot = m_insertionOrder;
    for (const uint32_t key : snapshot) {
        auto it = m_tiles.find(key);
        if (it != m_tiles.end()) {
            visitor(it->second);
        }
    }
}

const TileEntry* TileGrid::tileById(TileId id) const noexcept
{
    auto it = m_tiles.find(id.value);
    return (it != m_tiles.end()) ? &it->second : nullptr;
}

void TileGrid::recomputeCoverage(TileEntry& entry) const noexcept
{
    const double surfaceArea = static_cast<double>(m_surfaceSize.width())
                             * static_cast<double>(m_surfaceSize.height());
    if (surfaceArea <= 0.0) {
        entry.pixelCoverage = 0.0;
        return;
    }
    const double tileArea = entry.dst.width() * entry.dst.height();
    entry.pixelCoverage   = std::clamp(tileArea / surfaceArea, 0.0, 1.0);
}

} // namespace Kaivue::Render
