#include "map/CameraPlacementModel.h"

#include <QJsonParseError>

#include <cmath>
#include <limits>

namespace Kaivue::Map {

CameraPlacementModel::CameraPlacementModel(QObject* parent)
    : QObject(parent)
{
}

CameraPlacementModel::~CameraPlacementModel() = default;

int CameraPlacementModel::indexOf(const CameraId& id) const {
    for (int i = 0; i < m_items.size(); ++i) {
        if (m_items[i].id == id) {
            return i;
        }
    }
    return -1;
}

bool CameraPlacementModel::contains(const CameraId& id) const {
    return indexOf(id) >= 0;
}

const CameraPlacement* CameraPlacementModel::find(const CameraId& id) const {
    const int idx = indexOf(id);
    return idx < 0 ? nullptr : &m_items[idx];
}

void CameraPlacementModel::upsert(const CameraPlacement& p) {
    if (!p.id.isValid()) {
        return;
    }
    const int idx = indexOf(p.id);
    if (idx < 0) {
        m_items.append(p);
        emit placementAdded(p);
    } else {
        m_items[idx] = p;
        emit placementChanged(p);
    }
}

bool CameraPlacementModel::remove(const CameraId& id) {
    const int idx = indexOf(id);
    if (idx < 0) {
        return false;
    }
    m_items.removeAt(idx);
    emit placementRemoved(id);
    return true;
}

bool CameraPlacementModel::movePin(const CameraId& id, const QPointF& newPosition) {
    const int idx = indexOf(id);
    if (idx < 0) {
        return false;
    }
    m_items[idx].position = newPosition;
    emit placementChanged(m_items[idx]);
    return true;
}

void CameraPlacementModel::clear() {
    if (m_items.isEmpty()) {
        return;
    }
    m_items.clear();
    emit modelReset();
}

CameraId CameraPlacementModel::hitTest(const QPointF& point, qreal radius) const {
    if (radius <= 0.0) {
        return CameraId();
    }
    const qreal r2 = radius * radius;
    qreal best = std::numeric_limits<qreal>::max();
    CameraId winner;
    for (const auto& item : m_items) {
        const qreal dx = item.position.x() - point.x();
        const qreal dy = item.position.y() - point.y();
        const qreal d2 = dx * dx + dy * dy;
        if (d2 <= r2 && d2 < best) {
            best = d2;
            winner = item.id;
        }
    }
    return winner;
}

QJsonDocument CameraPlacementModel::toJson() const {
    QJsonArray arr;
    for (const auto& item : m_items) {
        QJsonObject obj;
        obj.insert(QStringLiteral("id"),    item.id.value);
        obj.insert(QStringLiteral("x"),     item.position.x());
        obj.insert(QStringLiteral("y"),     item.position.y());
        obj.insert(QStringLiteral("label"), item.label);
        obj.insert(QStringLiteral("rot"),   item.rotation);
        arr.append(obj);
    }
    QJsonObject root;
    root.insert(QStringLiteral("version"),    1);
    root.insert(QStringLiteral("placements"), arr);
    return QJsonDocument(root);
}

QByteArray CameraPlacementModel::toJsonBytes() const {
    return toJson().toJson(QJsonDocument::Compact);
}

bool CameraPlacementModel::loadFromJson(const QJsonDocument& doc) {
    if (!doc.isObject()) {
        return false;
    }
    const QJsonObject root = doc.object();
    if (root.value(QStringLiteral("version")).toInt(-1) != 1) {
        return false;
    }
    const QJsonValue arrVal = root.value(QStringLiteral("placements"));
    if (!arrVal.isArray()) {
        return false;
    }

    QVector<CameraPlacement> parsed;
    const QJsonArray arr = arrVal.toArray();
    parsed.reserve(arr.size());
    for (const QJsonValue& v : arr) {
        if (!v.isObject()) {
            return false;
        }
        const QJsonObject obj = v.toObject();
        const QString id = obj.value(QStringLiteral("id")).toString();
        if (id.isEmpty()) {
            return false;
        }
        CameraPlacement p;
        p.id       = CameraId(id);
        p.position = QPointF(obj.value(QStringLiteral("x")).toDouble(),
                             obj.value(QStringLiteral("y")).toDouble());
        p.label    = obj.value(QStringLiteral("label")).toString();
        p.rotation = obj.value(QStringLiteral("rot")).toDouble(0.0);
        parsed.append(p);
    }

    m_items = std::move(parsed);
    emit modelReset();
    return true;
}

bool CameraPlacementModel::loadFromJsonBytes(const QByteArray& bytes) {
    QJsonParseError err{};
    const QJsonDocument doc = QJsonDocument::fromJson(bytes, &err);
    if (err.error != QJsonParseError::NoError) {
        return false;
    }
    return loadFromJson(doc);
}

} // namespace Kaivue::Map
