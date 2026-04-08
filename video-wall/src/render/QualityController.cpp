#include "QualityController.h"

namespace Kaivue::Render {

QualityController::QualityController(QObject* parent)
    : QObject(parent)
{
    qRegisterMetaType<TileId>("Kaivue::Render::TileId");
    qRegisterMetaType<Quality>("Kaivue::Render::Quality");
}

void QualityController::evaluate(const TileGrid& grid)
{
    grid.forEach([this](const TileEntry& entry) {
        const Quality prev = currentQuality(entry.id);
        const Quality next = hintForCoverage(entry.pixelCoverage, prev);
        if (next != prev) {
            m_current[entry.id.value] = next;
            emit qualityHintChanged(entry.id, next);
        }
    });
}

Quality QualityController::currentQuality(TileId id) const noexcept
{
    auto it = m_current.find(id.value);
    return (it != m_current.end()) ? it->second : Quality::Low;
}

// static
Quality QualityController::hintForCoverage(double coverage, Quality current) noexcept
{
    switch (current) {
    case Quality::Low:
        if (coverage >= kLowToMedRise)  return Quality::Med;
        return Quality::Low;

    case Quality::Med:
        if (coverage >= kMedToHighRise) return Quality::High;
        if (coverage <  kMedToLowFall)  return Quality::Low;
        return Quality::Med;

    case Quality::High:
        if (coverage <  kHighToMedFall) return Quality::Med;
        return Quality::High;
    }
    return Quality::Low; // unreachable; suppresses -Wreturn-type
}

} // namespace Kaivue::Render
