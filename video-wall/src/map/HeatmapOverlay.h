#pragma once

//
// HeatmapOverlay.h — translucent intensity rendering (KAI-338 stub).
//
// Given a CameraId -> intensity [0.0, 1.0] map, paints gradient circles
// over a target QPaintDevice using each pin's current position. This is
// an operator-facing "attention" hint (people-count, alarm density, AI
// event rate) — not a scientific visualization; radius and gradient
// coefficients are fixed for v1 and will be tunable post-MVP.
//
// Rendering is CPU-only so the overlay can run in unit tests against a
// QImage target without a GPU.
//

#include "map/MapTypes.h"

#include <QColor>
#include <QHash>
#include <QImage>
#include <QPainter>
#include <QPointF>
#include <QRectF>

namespace Kaivue::Map {

class CameraPlacementModel;

class HeatmapOverlay {
public:
    HeatmapOverlay();
    ~HeatmapOverlay() = default;

    /**
     * Replace the current intensity snapshot. Values outside [0, 1] are
     * clamped. CameraIds not present in the placement model are ignored
     * at paint time.
     */
    void setIntensities(const QHash<CameraId, float>& values);

    /// Current (clamped) intensity for `id`; returns 0 if absent.
    [[nodiscard]] float intensityFor(const CameraId& id) const;

    /// Number of intensity samples currently loaded.
    [[nodiscard]] int sampleCount() const noexcept { return m_values.size(); }

    /**
     * Per-pin radius in target-device pixels. Defaults to 48 px.
     * Radius scales linearly with intensity so low-intensity pins still
     * render at ~25% of the maximum radius to remain visible.
     */
    void setBaseRadius(qreal px) noexcept { m_baseRadius = px; }
    [[nodiscard]] qreal baseRadius() const noexcept { return m_baseRadius; }

    /**
     * Paint the heatmap gradient circles for every placement in `model`
     * that has a non-zero intensity. Coordinates in `model` are already
     * assumed to be in the same coordinate system as `painter`'s target.
     * Returns the number of circles actually drawn (useful for tests).
     */
    int paint(QPainter& painter, const CameraPlacementModel& model) const;

    /**
     * Convenience: paint to an offscreen QImage of the given size.
     * The image is filled with transparent black before painting.
     */
    [[nodiscard]] QImage renderToImage(const QSize& size,
                                       const CameraPlacementModel& model) const;

private:
    QHash<CameraId, float> m_values;
    qreal                  m_baseRadius{48.0};
};

} // namespace Kaivue::Map
