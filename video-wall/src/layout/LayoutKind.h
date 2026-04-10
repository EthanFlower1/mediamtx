#pragma once

#include <QString>

namespace Kaivue::Layout {

/**
 * Layout templates supported per monitor.
 *
 * Grid4x4    — 16 equal tiles, 4 columns × 4 rows
 * Grid6x6    — 36 equal tiles
 * Grid9x16   — 9 tiles arranged 3×3 inside a 16:9 monitor (legacy SOC look)
 * PictureInPicture — 1 large background tile + 1 small overlay tile
 * Focus      — 1 enlarged "focus" tile + remaining tiles in a side strip
 */
enum class LayoutKind : int {
    Grid4x4 = 0,
    Grid6x6,
    Grid9x16,
    PictureInPicture,
    Focus,
};

inline int tileCountFor(LayoutKind kind) noexcept {
    switch (kind) {
    case LayoutKind::Grid4x4:          return 16;
    case LayoutKind::Grid6x6:          return 36;
    case LayoutKind::Grid9x16:         return 9;
    case LayoutKind::PictureInPicture: return 2;
    case LayoutKind::Focus:            return 8;
    }
    return 0;
}

inline QString layoutKindName(LayoutKind kind) {
    switch (kind) {
    case LayoutKind::Grid4x4:          return QStringLiteral("Grid4x4");
    case LayoutKind::Grid6x6:          return QStringLiteral("Grid6x6");
    case LayoutKind::Grid9x16:         return QStringLiteral("Grid9x16");
    case LayoutKind::PictureInPicture: return QStringLiteral("PictureInPicture");
    case LayoutKind::Focus:            return QStringLiteral("Focus");
    }
    return QStringLiteral("Unknown");
}

inline bool layoutKindFromName(const QString& name, LayoutKind& out) {
    if (name == QLatin1String("Grid4x4"))          { out = LayoutKind::Grid4x4;          return true; }
    if (name == QLatin1String("Grid6x6"))          { out = LayoutKind::Grid6x6;          return true; }
    if (name == QLatin1String("Grid9x16"))         { out = LayoutKind::Grid9x16;         return true; }
    if (name == QLatin1String("PictureInPicture")) { out = LayoutKind::PictureInPicture; return true; }
    if (name == QLatin1String("Focus"))            { out = LayoutKind::Focus;            return true; }
    return false;
}

} // namespace Kaivue::Layout
