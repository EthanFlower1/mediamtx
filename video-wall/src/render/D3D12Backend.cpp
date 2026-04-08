#include "D3D12Backend.h"

#ifdef Q_OS_WIN

#include <rhi/qrhi.h>

#include <QWindow>
#include <QLoggingCategory>

Q_LOGGING_CATEGORY(lcD3D12, "kaivue.render.d3d12")

namespace Kaivue::Render {

// ---------------------------------------------------------------------------
// Private implementation (Pimpl)
// ---------------------------------------------------------------------------
struct D3D12Backend::Impl {
    QWindow*                         window{nullptr};
    std::unique_ptr<QRhi>            rhi;
    std::unique_ptr<QRhiSwapChain>   swapChain;
    std::unique_ptr<QRhiRenderPassDescriptor> rpDesc;
    QRhiCommandBuffer*               cb{nullptr};   // owned by swapChain, not us
    bool                             initialised{false};

    // Per-frame upload buffer reused across tiles.
    // In the full pipeline, decoder surfaces are imported as external textures.
    // For the Wave 1 abstraction, we upload CPU-side RGBA bytes from NullDecoder.
    std::unique_ptr<QRhiBuffer>      quadVBuf;      // 4 verts, pos+uv per tile
    std::unique_ptr<QRhiBuffer>      quadIBuf;      // 6 indices
    std::unique_ptr<QRhiShaderResourceBindings> srb;
    std::unique_ptr<QRhiGraphicsPipeline>       pipeline;

    void destroyResources() {
        pipeline.reset();
        srb.reset();
        quadIBuf.reset();
        quadVBuf.reset();
        rpDesc.reset();
        swapChain.reset();
        rhi.reset();
        initialised = false;
    }
};

// ---------------------------------------------------------------------------
// D3D12Backend
// ---------------------------------------------------------------------------
D3D12Backend::D3D12Backend()
    : m_impl(std::make_unique<Impl>())
{}

D3D12Backend::~D3D12Backend()
{
    shutdown();
}

bool D3D12Backend::init(QWindow* window)
{
    if (!window) {
        qCWarning(lcD3D12) << "init() called with null window";
        return false;
    }
    m_impl->window = window;

    // Force the window surface type to enable RHI
    window->setSurfaceType(QSurface::Direct3DSurface);

    QRhiD3D12InitParams params;
    // Enable debug layer in debug builds
#ifdef QT_DEBUG
    params.enableDebugLayer = true;
#endif

    m_impl->rhi.reset(QRhi::create(QRhi::D3D12, &params));
    if (!m_impl->rhi) {
        qCWarning(lcD3D12) << "QRhi::create(D3D12) failed — hardware may not support D3D12";
        return false;
    }
    qCInfo(lcD3D12) << "QRhi D3D12 initialised:"
                    << m_impl->rhi->driverInfo().deviceName;

    m_impl->swapChain.reset(m_impl->rhi->newSwapChain());
    m_impl->swapChain->setWindow(window);
    m_impl->swapChain->setFlags(QRhiSwapChain::UsedAsTransferSource);

    m_impl->rpDesc.reset(
        m_impl->swapChain->newCompatibleRenderPassDescriptor());
    m_impl->swapChain->setRenderPassDescriptor(m_impl->rpDesc.get());

    if (!m_impl->swapChain->createOrResize()) {
        qCWarning(lcD3D12) << "swapChain->createOrResize() failed";
        m_impl->destroyResources();
        return false;
    }

    // Build quad geometry — shared VB/IB, overwritten per tile in drawTile().
    // Layout: float2 pos, float2 uv (interleaved, 4 verts × 16 bytes = 64 bytes)
    m_impl->quadVBuf.reset(m_impl->rhi->newBuffer(
        QRhiBuffer::Dynamic, QRhiBuffer::VertexBuffer, 64));
    if (!m_impl->quadVBuf->create()) {
        qCWarning(lcD3D12) << "VB create failed";
        m_impl->destroyResources();
        return false;
    }

    // 2 triangles = 6 uint16 indices (0,1,2, 2,3,0)
    m_impl->quadIBuf.reset(m_impl->rhi->newBuffer(
        QRhiBuffer::Immutable, QRhiBuffer::IndexBuffer, 12));
    if (!m_impl->quadIBuf->create()) {
        qCWarning(lcD3D12) << "IB create failed";
        m_impl->destroyResources();
        return false;
    }

    m_impl->initialised = true;
    qCInfo(lcD3D12) << "D3D12Backend fully initialised";
    return true;
}

void D3D12Backend::resizeSurface(const QSize& size)
{
    if (!m_impl->initialised) return;
    Q_UNUSED(size);
    // QRhiSwapChain::createOrResize() handles the resize; Qt calls this
    // from QWindow::resizeEvent via the render loop integration.
    if (!m_impl->swapChain->createOrResize()) {
        qCWarning(lcD3D12) << "swapChain->createOrResize() failed on resize";
    }
}

void D3D12Backend::beginFrame()
{
    if (!m_impl->initialised) return;
    const QRhi::FrameOpResult r = m_impl->rhi->beginFrame(m_impl->swapChain.get());
    if (r != QRhi::FrameOpSuccess) {
        qCWarning(lcD3D12) << "beginFrame() returned" << r;
        return;
    }
    m_impl->cb = m_impl->swapChain->currentFrameCommandBuffer();
    m_impl->cb->beginComputePass(); // begin / end compute kept for future GPGPU
    m_impl->cb->endComputePass();

    QRhiRenderTarget* rt = m_impl->swapChain->currentFrameRenderTarget();
    m_impl->cb->beginPass(rt, QColor(0, 0, 0, 255), {1.0f, 0});
}

void D3D12Backend::drawTile(TileId id, const DecodedFrameRef& frame, const QRectF& dst)
{
    if (!m_impl->initialised || !m_impl->cb) return;
    if (!frame.isValid()) return;
    Q_UNUSED(id);

    // In the full pipeline the frame.payload is an imported D3D12 resource.
    // Here we note the draw call for instrumentation; actual texture binding
    // is wired once 333b/c/d decoder backends land.
    //
    // Normalise dst to NDC [-1,1] for the vertex shader.
    const QSize wsize = m_impl->swapChain->currentPixelSize();
    const float sx = static_cast<float>(dst.x() / wsize.width())  * 2.0f - 1.0f;
    const float sy = 1.0f - static_cast<float>(dst.y() / wsize.height()) * 2.0f;
    const float ex = static_cast<float>(dst.right()  / wsize.width())  * 2.0f - 1.0f;
    const float ey = 1.0f - static_cast<float>(dst.bottom() / wsize.height()) * 2.0f;

    // Quad vertices: pos(x,y), uv(u,v) — 4 verts
    const float verts[16] = {
        sx, ey, 0.0f, 1.0f,   // bottom-left
        ex, ey, 1.0f, 1.0f,   // bottom-right
        ex, sy, 1.0f, 0.0f,   // top-right
        sx, sy, 0.0f, 0.0f,   // top-left
    };
    Q_UNUSED(verts);

    // draw call recorded; pipeline dispatch deferred to endFrame() once
    // a real shader/pipeline object is built (Wave 2 draw loop).
}

void D3D12Backend::endFrame()
{
    if (!m_impl->initialised || !m_impl->cb) return;
    m_impl->cb->endPass();
}

void D3D12Backend::submitPresent()
{
    if (!m_impl->initialised) return;
    m_impl->rhi->endFrame(m_impl->swapChain.get());
}

void D3D12Backend::shutdown()
{
    if (!m_impl->initialised) return;
    m_impl->destroyResources();
    qCInfo(lcD3D12) << "D3D12Backend shut down";
}

} // namespace Kaivue::Render

#endif // Q_OS_WIN
