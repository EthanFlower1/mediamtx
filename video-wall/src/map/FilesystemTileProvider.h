#pragma once

//
// FilesystemTileProvider.h — air-gapped slippy tile source (KAI-338).
//
// Loads tiles from a pre-staged directory using the canonical layout
//   <root>/<z>/<x>/<y>.png
// This is the format produced by tools like mb-util, josm tile exporters,
// and the Kaivue offline map packager.
//
// All I/O is file-local; the provider never touches the network, making
// it safe for SOC operator workstations that live behind an air gap.
//

#include "map/ITileProvider.h"

#include <QCache>
#include <QMutex>
#include <QString>

#include <memory>

namespace Kaivue::Map {

class FilesystemTileProvider final : public ITileProvider {
public:
    /**
     * @param rootDir     Directory containing a `z/x/y.png` hierarchy.
     * @param minZoom     Lowest zoom level shipped by the tile bundle.
     * @param maxZoom     Highest zoom level shipped by the tile bundle.
     * @param cacheBytes  Decoded-PNG cache budget (default 64 MiB).
     */
    explicit FilesystemTileProvider(QString rootDir,
                                    int minZoom = 0,
                                    int maxZoom = 19,
                                    int cacheBytes = 64 * 1024 * 1024);

    ~FilesystemTileProvider() override;

    [[nodiscard]] bool    fetchTile(const TileKey& key, QByteArray& outPng) override;
    [[nodiscard]] QString providerName() const override;
    [[nodiscard]] int     minZoom() const override { return m_minZoom; }
    [[nodiscard]] int     maxZoom() const override { return m_maxZoom; }

    /**
     * Returns the absolute filesystem path that would be consulted for
     * the given tile key. Exposed for tests and offline debugging.
     */
    [[nodiscard]] QString tilePath(const TileKey& key) const;

private:
    QString m_rootDir;
    int     m_minZoom;
    int     m_maxZoom;

    // Small decoded-byte cache keyed on "z/x/y".
    mutable QMutex                    m_cacheMutex;
    mutable QCache<QString, QByteArray> m_cache;
};

} // namespace Kaivue::Map
