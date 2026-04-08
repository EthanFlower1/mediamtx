#pragma once

#include <cstdint>
#include <cstddef>

namespace Kaivue::Render {

/**
 * Lightweight strong-typed tile identifier.
 * Wraps uint32_t to prevent implicit conversion from raw integers.
 */
struct TileId {
    uint32_t value{0};

    constexpr TileId() noexcept = default;
    constexpr explicit TileId(uint32_t v) noexcept : value(v) {}

    constexpr bool operator==(const TileId& other) const noexcept { return value == other.value; }
    constexpr bool operator!=(const TileId& other) const noexcept { return value != other.value; }
    constexpr bool operator<(const TileId& other)  const noexcept { return value < other.value; }
};

/**
 * Opaque decoded-frame handle passed from decoder sub-systems (333b/c/d)
 * to the render backend.
 *
 * Public fields are readable by the render backend for framing decisions.
 * The opaque `payload` pointer is interpreted by each backend:
 *   - NVDEC (333b):         CUdeviceptr (CUDA device memory)
 *   - D3D12/QuickSync win (333c): ID3D12Resource*
 *   - VA-API/QuickSync linux:    VASurfaceID stored as uintptr_t
 *   - AMD AMF (333d):            amf::AMFSurface*
 *   - NullDecoder:               RGBA uint8_t* (CPU)
 *
 * Lifetime: owned by the decoder that produced it; render backend must
 * not retain the pointer past the current frame's endFrame() call.
 */
struct DecodedFrameRef {
    int          width{0};
    int          height{0};
    std::int64_t pts{0};    // presentation timestamp, microseconds
    void*        payload{nullptr};  // platform-specific; see above

    [[nodiscard]] bool isValid() const noexcept {
        return width > 0 && height > 0 && payload != nullptr;
    }
};

} // namespace Kaivue::Render
