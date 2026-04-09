#pragma once

#include "MonitorId.h"

#include <QHash>
#include <QString>

#include <cstdint>
#include <vector>

namespace Kaivue::Layout {

/**
 * Strong-typed Salvo identifier.
 */
struct SalvoId {
    uint32_t value{0};

    constexpr SalvoId() noexcept = default;
    constexpr explicit SalvoId(uint32_t v) noexcept : value(v) {}

    constexpr bool operator==(const SalvoId& other) const noexcept { return value == other.value; }
    constexpr bool operator!=(const SalvoId& other) const noexcept { return value != other.value; }
    constexpr bool operator<(const SalvoId& other)  const noexcept { return value < other.value; }
};

/**
 * Salvo: a one-button "flip every monitor to these specific cameras" preset.
 *
 * The MonitorLayout's existing template (kind) is preserved; only the
 * camera assignments are overwritten.  Operators bind salvos to a hard
 * key on the Falcon keyboard for instant scene-without-template-change.
 */
struct Salvo {
    SalvoId                              id{};
    QString                              name;
    QHash<MonitorId, std::vector<CameraId>> cameras_per_monitor{};
};

} // namespace Kaivue::Layout
