#pragma once

#include "IRenderBackend.h"

#include <memory>
#include <vector>
#include <unordered_map>

namespace Kaivue::Render {

/**
 * NullDecoderBackend — CPU-only software rasterizer backend.
 *
 * Produces a deterministic 8x8 checker-pattern RGBA texture per tile.
 * The checker squares alternate between the tile's foreground colour
 * (derived from TileId) and #333333 grey.
 *
 * Used by:
 *   - Unit tests (QOffscreenSurface + QRhi::Null or QRhi::OpenGL)
 *   - CI pipelines (no real GPU present)
 *   - Development machines without a supported GPU
 *
 * The checker pattern is byte-for-byte deterministic given the same
 * TileId and frame dimensions, enabling golden-hash assertions.
 *
 * frame.payload points to an internally-managed RGBA uint8_t buffer.
 * The buffer is valid until the next call to drawTile() for the same TileId
 * or until shutdown().
 */
class NullDecoderBackend : public IRenderBackend {
    Q_DISABLE_COPY_MOVE(NullDecoderBackend)
public:
    static constexpr int kDefaultWidth  = 320;
    static constexpr int kDefaultHeight = 180;
    static constexpr int kCheckerSize   = 8; // checker square side length in pixels

    NullDecoderBackend();
    ~NullDecoderBackend() override;

    bool init(QWindow* window) override;
    void resizeSurface(const QSize& size) override;
    void beginFrame() override;
    void drawTile(TileId id, const DecodedFrameRef& frame, const QRectF& dst) override;
    void endFrame() override;
    void submitPresent() override;
    void shutdown() override;

    /**
     * Generate a DecodedFrameRef carrying a checker-pattern RGBA buffer.
     * Caller owns nothing — the frame is valid until the next generateFrame()
     * call for the same TileId.
     *
     * @param id     TileId used to derive the checker foreground colour.
     * @param width  Frame width in pixels.
     * @param height Frame height in pixels.
     * @param pts    Presentation timestamp (microseconds).
     */
    DecodedFrameRef generateFrame(TileId id, int width, int height, std::int64_t pts);

    /**
     * Compute a simple FNV-1a hash over the RGBA buffer of the last
     * generateFrame() result for the given TileId.
     * Returns 0 if the TileId is unknown.
     */
    [[nodiscard]] uint32_t pixelHash(TileId id) const noexcept;

private:
    struct TileBuffer {
        std::vector<uint8_t> rgba;  // width * height * 4 bytes
        int                  width{0};
        int                  height{0};
    };

    static void fillChecker(TileBuffer& buf, TileId id) noexcept;
    static uint32_t fnv1a(const uint8_t* data, std::size_t len) noexcept;

    std::unordered_map<uint32_t, TileBuffer> m_buffers;
    QSize                                    m_surfaceSize{1920, 1080};
    bool                                     m_initialised{false};
    int                                      m_frameCount{0};
};

} // namespace Kaivue::Render
