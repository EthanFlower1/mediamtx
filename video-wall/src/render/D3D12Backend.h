#pragma once

#ifdef Q_OS_WIN

#include "IRenderBackend.h"

#include <memory>

// Qt RHI private headers — requires Qt6::GuiPrivate
#include <rhi/qrhi.h>

namespace Kaivue::Render {

/**
 * Windows D3D12 render backend via Qt RHI.
 *
 * Requires Qt 6.6+ (QRhi::D3D12 backend was promoted to stable in 6.6).
 * Link against Qt6::GuiPrivate for <rhi/qrhi.h>.
 *
 * This implementation:
 *   - Creates a QRhi instance in D3D12 mode.
 *   - Maintains a single render pass that iterates all tiles.
 *   - For each tile, binds the DecodedFrameRef payload as a QRhiTexture
 *     (type ExternalOES or RGBA8 for NullDecoder frames).
 *   - Draws a textured quad covering the dst rectangle via a minimal
 *     vertex/fragment shader pipeline (no z-buffer needed).
 *   - Presents via QRhiSwapChain.
 *
 * Decoder sub-tickets (333b/c/d) will import their GPU surfaces here via
 * QRhiTexture::createFrom(NativeTexture) once those backends land.
 */
class D3D12Backend : public IRenderBackend {
    Q_DISABLE_COPY_MOVE(D3D12Backend)
public:
    D3D12Backend();
    ~D3D12Backend() override;

    bool init(QWindow* window) override;
    void resizeSurface(const QSize& size) override;
    void beginFrame() override;
    void drawTile(TileId id, const DecodedFrameRef& frame, const QRectF& dst) override;
    void endFrame() override;
    void submitPresent() override;
    void shutdown() override;

private:
    struct Impl;
    std::unique_ptr<Impl> m_impl;
};

} // namespace Kaivue::Render

#endif // Q_OS_WIN
