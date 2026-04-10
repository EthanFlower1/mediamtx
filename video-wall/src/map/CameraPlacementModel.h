#pragma once

//
// CameraPlacementModel.h — owns camera pins for the map view (KAI-338).
//
// Stores an ordered collection of CameraPlacement records and provides:
//   - JSON serialization (load/save, round-trip safe)
//   - point-radius hit testing (used by MapView for click detection)
//   - observable mutations (Qt signals) so both the Quick item and the
//     heatmap overlay can stay in sync without explicit refreshes.
//
// The model is UI-mode-agnostic: callers interpret QPointF either as
// {longitude, latitude} (Geographic) or {x, y} scene pixels (FloorPlan).
//

#include "map/MapTypes.h"

#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QObject>
#include <QPointF>
#include <QString>
#include <QVector>

namespace Kaivue::Map {

class CameraPlacementModel : public QObject {
    Q_OBJECT
public:
    explicit CameraPlacementModel(QObject* parent = nullptr);
    ~CameraPlacementModel() override;

    // ------------------------------------------------------------------
    // Mutations
    // ------------------------------------------------------------------

    /**
     * Insert or replace the placement for the given camera. If a pin
     * with the same CameraId already exists, it is updated in place.
     * Emits placementChanged() on replace and placementAdded() on insert.
     */
    void upsert(const CameraPlacement& p);

    /**
     * Remove the placement for `id`. No-op if not present.
     * Emits placementRemoved() on success.
     */
    bool remove(const CameraId& id);

    /**
     * Move an existing pin to a new position without touching label/rotation.
     * @return true if the pin existed and was moved.
     */
    bool movePin(const CameraId& id, const QPointF& newPosition);

    /**
     * Clear all pins. Emits modelReset().
     */
    void clear();

    // ------------------------------------------------------------------
    // Read access
    // ------------------------------------------------------------------

    [[nodiscard]] int count() const noexcept { return m_items.size(); }
    [[nodiscard]] const QVector<CameraPlacement>& items() const noexcept { return m_items; }
    [[nodiscard]] bool contains(const CameraId& id) const;
    [[nodiscard]] const CameraPlacement* find(const CameraId& id) const;

    // ------------------------------------------------------------------
    // Hit testing
    // ------------------------------------------------------------------

    /**
     * Return the CameraId of the pin whose center is within `radius`
     * scene/world units of `point`. Returns invalid CameraId if no hit.
     *
     * When multiple pins match, the one closest to `point` wins. Used
     * by MapView click routing and by QTest synthetic events.
     */
    [[nodiscard]] CameraId hitTest(const QPointF& point, qreal radius) const;

    // ------------------------------------------------------------------
    // Serialization
    // ------------------------------------------------------------------

    /**
     * Serialize to a compact JSON document.
     *
     * Schema:
     *   {
     *     "version": 1,
     *     "placements": [
     *       { "id": "...", "x": <num>, "y": <num>, "label": "...", "rot": <num> },
     *       ...
     *     ]
     *   }
     */
    [[nodiscard]] QJsonDocument toJson() const;
    [[nodiscard]] QByteArray    toJsonBytes() const;

    /**
     * Replace the current contents with data parsed from `doc`.
     * @return true on success; false if the document is malformed.
     *              On failure the model is left unchanged.
     */
    bool loadFromJson(const QJsonDocument& doc);
    bool loadFromJsonBytes(const QByteArray& bytes);

signals:
    void placementAdded(const Kaivue::Map::CameraPlacement& p);
    void placementChanged(const Kaivue::Map::CameraPlacement& p);
    void placementRemoved(const Kaivue::Map::CameraId& id);
    void modelReset();

private:
    [[nodiscard]] int indexOf(const CameraId& id) const;

    QVector<CameraPlacement> m_items;
};

} // namespace Kaivue::Map
