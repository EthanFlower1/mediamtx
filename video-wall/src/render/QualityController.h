#pragma once

#include "DecodedFrame.h"
#include "TileGrid.h"

#include <QObject>

#include <unordered_map>

namespace Kaivue::Render {

/**
 * Quality hint emitted to decoder sub-systems.
 *
 * Decoders (333b/c/d) will subscribe to qualityHintChanged() and switch
 * the sub-stream (high/medium/low resolution) accordingly.
 */
enum class Quality : uint8_t {
    Low  = 0,   // tile covers < 10 % of surface; use lowest sub-stream
    Med  = 1,   // 10–40 %
    High = 2,   // > 40 %
};

/**
 * QualityController — watches TileGrid pixel coverage and emits
 * Quality hint signals whenever coverage crosses a threshold.
 *
 * Thresholds (hysteresis applied to avoid oscillation):
 *   LOW  -> MED  : coverage crosses above  10 %
 *   MED  -> HIGH : coverage crosses above  40 %
 *   HIGH -> MED  : coverage drops  below   35 %
 *   MED  -> LOW  : coverage drops  below    8 %
 *
 * Call evaluate() once per frame after TileGrid has been updated.
 */
class QualityController : public QObject {
    Q_OBJECT
public:
    explicit QualityController(QObject* parent = nullptr);
    ~QualityController() override = default;

    /**
     * Evaluate quality hints for all tiles in the grid.
     * Emits qualityHintChanged() for each tile whose quality level changes.
     */
    void evaluate(const TileGrid& grid);

    /**
     * Current quality hint for a tile.  Returns Low if unknown.
     */
    [[nodiscard]] Quality currentQuality(TileId id) const noexcept;

signals:
    /**
     * Emitted when a tile's quality hint transitions.
     * Decoder backends connect to this signal to select the appropriate
     * sub-stream bitrate / resolution.
     */
    void qualityHintChanged(Kaivue::Render::TileId id, Kaivue::Render::Quality quality);

private:
    [[nodiscard]] static Quality hintForCoverage(double coverage,
                                                 Quality current) noexcept;

    std::unordered_map<uint32_t, Quality> m_current;

    // Thresholds (rise / fall with hysteresis)
    static constexpr double kLowToMedRise  = 0.10;
    static constexpr double kMedToHighRise = 0.40;
    static constexpr double kHighToMedFall = 0.35;
    static constexpr double kMedToLowFall  = 0.08;
};

} // namespace Kaivue::Render

// Make TileId and Quality usable as QMetaType arguments across signal/slot connections.
Q_DECLARE_METATYPE(Kaivue::Render::TileId)
Q_DECLARE_METATYPE(Kaivue::Render::Quality)
