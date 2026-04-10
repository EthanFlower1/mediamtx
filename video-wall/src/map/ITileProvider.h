#pragma once

//
// ITileProvider.h — pluggable slippy-map tile source (KAI-338).
//
// Air-gapped SOC deployments must never reach the public internet; the
// map view therefore loads tiles via an ITileProvider abstraction. Two
// concrete providers ship with the scaffold:
//
//   * FilesystemTileProvider — reads z/x/y.png from a pre-staged directory
//   * NullTileProvider       — returns empty tiles, used by unit tests
//
// A future provider (e.g. embedded MBTiles) can drop in without touching
// any call sites.
//

#include <QByteArray>
#include <QString>

namespace Kaivue::Map {

/**
 * Key identifying a single slippy-map tile.
 *
 * Coordinates follow the standard OSM/Google tile scheme:
 *   z  = zoom level (0 = world, 19 = street)
 *   x  = column index [0, 2^z)
 *   y  = row index    [0, 2^z)  — origin top-left
 */
struct TileKey {
    int z{0};
    int x{0};
    int y{0};

    [[nodiscard]] bool isValid() const noexcept {
        if (z < 0 || z > 22) {
            return false;
        }
        const int span = 1 << z;
        return x >= 0 && x < span && y >= 0 && y < span;
    }

    bool operator==(const TileKey& other) const noexcept {
        return z == other.z && x == other.x && y == other.y;
    }
};

/**
 * Abstract tile provider.
 *
 * Implementations MUST be thread-safe: the Qt Location / Quick engine may
 * call fetchTile() from background threads.
 */
class ITileProvider {
public:
    virtual ~ITileProvider() = default;

    /**
     * Fetch a single tile synchronously.
     *
     * @param key     Tile coordinate.
     * @param outPng  On success, receives the raw PNG bytes.
     * @return true if the tile was found; false otherwise (caller should
     *              render a placeholder).
     *
     * Must not block on network I/O. Implementations are expected to be
     * local-only (filesystem, memory cache, embedded MBTiles, etc.).
     */
    [[nodiscard]] virtual bool fetchTile(const TileKey& key, QByteArray& outPng) = 0;

    /**
     * Human-readable identifier for logs and telemetry.
     */
    [[nodiscard]] virtual QString providerName() const = 0;

    /**
     * Declared zoom range the provider can serve. The view clamps user
     * interaction to [minZoom(), maxZoom()].
     */
    [[nodiscard]] virtual int minZoom() const = 0;
    [[nodiscard]] virtual int maxZoom() const = 0;
};

} // namespace Kaivue::Map
