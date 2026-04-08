#pragma once

#include "DecodedFrame.h"

#include <QRectF>
#include <QSize>

#include <cstdint>
#include <functional>
#include <unordered_map>
#include <vector>

namespace Kaivue::Render {

/**
 * Describes one tile slot in the grid.
 */
struct TileEntry {
    TileId         id;
    DecodedFrameRef frame;
    QRectF         dst;           // pixel coordinates on the surface
    double         pixelCoverage; // [0.0, 1.0] fraction of surface this tile covers
};

/**
 * TileGrid — owns up to 64 tile slots.
 *
 * Responsibilities:
 *   - Tracks the current frame + destination rect per tile.
 *   - Computes pixelCoverage = (tile area) / (surface area) each time
 *     the surface size or tile rect changes.
 *   - Provides a stable iteration interface that is safe against
 *     removeTile() calls made from within the visitor lambda.
 *
 * Not thread-safe: call from the render thread only.
 */
class TileGrid {
public:
    static constexpr int kMaxTiles = 64;

    TileGrid() = default;
    ~TileGrid() = default;

    // Non-copyable; movable.
    TileGrid(const TileGrid&) = delete;
    TileGrid& operator=(const TileGrid&) = delete;
    TileGrid(TileGrid&&) = default;
    TileGrid& operator=(TileGrid&&) = default;

    /**
     * Set the surface size used for pixel-coverage computation.
     * Call whenever the window is resized.
     */
    void setSurfaceSize(const QSize& size);

    /**
     * Insert or update a tile.  If TileId already exists, replaces it.
     * @throws nothing — silently drops if id.value >= kMaxTiles (should never happen).
     */
    void setTile(TileId id, const DecodedFrameRef& frame, const QRectF& dst);

    /**
     * Remove a tile.  No-op if not present.
     * Safe to call from inside forEach().
     */
    void removeTile(TileId id);

    /**
     * Number of active tiles.
     */
    [[nodiscard]] int tileCount() const noexcept;

    /**
     * Iterate over a snapshot of the current tiles.
     * The snapshot is taken before iteration so removeTile() during the
     * visitor is safe.
     *
     * @param visitor  Called for each TileEntry in insertion order.
     */
    void forEach(const std::function<void(const TileEntry&)>& visitor) const;

    /**
     * Direct tile access by id.  Returns nullptr if not present.
     */
    [[nodiscard]] const TileEntry* tileById(TileId id) const noexcept;

private:
    void recomputeCoverage(TileEntry& entry) const noexcept;

    std::unordered_map<uint32_t, TileEntry> m_tiles;
    std::vector<uint32_t>                   m_insertionOrder; // preserves draw order
    QSize                                   m_surfaceSize{1920, 1080};
};

} // namespace Kaivue::Render
