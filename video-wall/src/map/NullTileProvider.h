#pragma once

//
// NullTileProvider.h — test-only tile source (KAI-338).
//
// Returns empty tiles for all keys; used by CI unit tests and headless
// verification runs where no filesystem bundle is available. Records
// every fetch for assertion purposes.
//

#include "map/ITileProvider.h"

#include <QVector>

namespace Kaivue::Map {

class NullTileProvider final : public ITileProvider {
public:
    NullTileProvider() = default;
    ~NullTileProvider() override = default;

    [[nodiscard]] bool fetchTile(const TileKey& key, QByteArray& outPng) override {
        m_fetched.append(key);
        outPng.clear();
        return false;
    }

    [[nodiscard]] QString providerName() const override {
        return QStringLiteral("null");
    }

    [[nodiscard]] int minZoom() const override { return 0; }
    [[nodiscard]] int maxZoom() const override { return 19; }

    /// Every key requested since construction (test introspection).
    [[nodiscard]] const QVector<TileKey>& fetchedKeys() const noexcept {
        return m_fetched;
    }

    void clearFetchLog() noexcept { m_fetched.clear(); }

private:
    QVector<TileKey> m_fetched;
};

} // namespace Kaivue::Map
