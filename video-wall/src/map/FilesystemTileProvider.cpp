#include "map/FilesystemTileProvider.h"

#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QMutexLocker>

namespace Kaivue::Map {

FilesystemTileProvider::FilesystemTileProvider(QString rootDir,
                                               int minZoom,
                                               int maxZoom,
                                               int cacheBytes)
    : m_rootDir(std::move(rootDir))
    , m_minZoom(minZoom)
    , m_maxZoom(maxZoom)
    , m_cache(cacheBytes)
{
}

FilesystemTileProvider::~FilesystemTileProvider() = default;

QString FilesystemTileProvider::providerName() const {
    return QStringLiteral("filesystem:%1").arg(m_rootDir);
}

QString FilesystemTileProvider::tilePath(const TileKey& key) const {
    return QStringLiteral("%1/%2/%3/%4.png")
        .arg(m_rootDir)
        .arg(key.z)
        .arg(key.x)
        .arg(key.y);
}

bool FilesystemTileProvider::fetchTile(const TileKey& key, QByteArray& outPng) {
    if (!key.isValid()) {
        return false;
    }
    if (key.z < m_minZoom || key.z > m_maxZoom) {
        return false;
    }

    const QString cacheKey =
        QStringLiteral("%1/%2/%3").arg(key.z).arg(key.x).arg(key.y);

    {
        QMutexLocker lock(&m_cacheMutex);
        if (auto* hit = m_cache.object(cacheKey)) {
            outPng = *hit;
            return !outPng.isEmpty();
        }
    }

    const QString path = tilePath(key);
    QFile f(path);
    if (!f.exists()) {
        return false;
    }
    if (!f.open(QIODevice::ReadOnly)) {
        return false;
    }
    outPng = f.readAll();
    f.close();

    {
        QMutexLocker lock(&m_cacheMutex);
        // QCache owns the inserted pointer; cost is bytes consumed.
        auto* copy = new QByteArray(outPng);
        if (!m_cache.insert(cacheKey, copy, outPng.size())) {
            // insertion refused (cost exceeds cache budget); that's fine.
        }
    }

    return !outPng.isEmpty();
}

} // namespace Kaivue::Map
