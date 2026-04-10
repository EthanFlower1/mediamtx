#include "map/HeatmapOverlay.h"

#include "map/CameraPlacementModel.h"

#include <QRadialGradient>

#include <algorithm>

namespace Kaivue::Map {

HeatmapOverlay::HeatmapOverlay() = default;

static float clamp01(float v) noexcept {
    if (v < 0.0f) return 0.0f;
    if (v > 1.0f) return 1.0f;
    return v;
}

void HeatmapOverlay::setIntensities(const QHash<CameraId, float>& values) {
    m_values.clear();
    m_values.reserve(values.size());
    for (auto it = values.cbegin(); it != values.cend(); ++it) {
        m_values.insert(it.key(), clamp01(it.value()));
    }
}

float HeatmapOverlay::intensityFor(const CameraId& id) const {
    return m_values.value(id, 0.0f);
}

int HeatmapOverlay::paint(QPainter& painter, const CameraPlacementModel& model) const {
    if (m_values.isEmpty() || m_baseRadius <= 0.0) {
        return 0;
    }

    const QPainter::RenderHints oldHints = painter.renderHints();
    painter.setRenderHint(QPainter::Antialiasing, true);
    painter.setPen(Qt::NoPen);

    int drawn = 0;
    for (const auto& item : model.items()) {
        const float intensity = m_values.value(item.id, 0.0f);
        if (intensity <= 0.0f) {
            continue;
        }
        // Radius lerps from 25% to 100% of base; keeps low-signal pins visible.
        const qreal radius = m_baseRadius * (0.25 + 0.75 * intensity);

        QRadialGradient grad(item.position, radius);
        // Hot center, transparent edge.
        const int hotAlpha = static_cast<int>(std::clamp(180.0f * intensity, 30.0f, 220.0f));
        grad.setColorAt(0.0, QColor(255,  40,   0, hotAlpha));
        grad.setColorAt(0.5, QColor(255, 180,   0, hotAlpha / 2));
        grad.setColorAt(1.0, QColor(255, 255,   0, 0));

        painter.setBrush(grad);
        painter.drawEllipse(item.position, radius, radius);
        ++drawn;
    }

    painter.setRenderHints(oldHints);
    return drawn;
}

QImage HeatmapOverlay::renderToImage(const QSize& size,
                                     const CameraPlacementModel& model) const {
    QImage img(size, QImage::Format_ARGB32_Premultiplied);
    img.fill(Qt::transparent);
    QPainter p(&img);
    paint(p, model);
    p.end();
    return img;
}

} // namespace Kaivue::Map
