#include "VulkanBackend.h"

#ifdef Q_OS_LINUX

#include <rhi/qrhi.h>

#include <QWindow>
#include <QVulkanInstance>
#include <QLoggingCategory>

Q_LOGGING_CATEGORY(lcVulkan, "kaivue.render.vulkan")

namespace Kaivue::Render {

// ---------------------------------------------------------------------------
// Private implementation (Pimpl)
// ---------------------------------------------------------------------------
struct VulkanBackend::Impl {
    QWindow*                         window{nullptr};
    std::unique_ptr<QRhi>            rhi;
    std::unique_ptr<QRhiSwapChain>   swapChain;
    std::unique_ptr<QRhiRenderPassDescriptor> rpDesc;
    QRhiCommandBuffer*               cb{nullptr};
    bool                             initialised{false};

    // Shared quad geometry (see D3D12Backend for layout notes)
    std::unique_ptr<QRhiBuffer>      quadVBuf;
    std::unique_ptr<QRhiBuffer>      quadIBuf;

    void destroyResources() {
        quadIBuf.reset();
        quadVBuf.reset();
        rpDesc.reset();
        swapChain.reset();
        rhi.reset();
        initialised = false;
    }
};

// ---------------------------------------------------------------------------
// VulkanBackend
// ---------------------------------------------------------------------------
VulkanBackend::VulkanBackend()
    : m_impl(std::make_unique<Impl>())
{}

VulkanBackend::~VulkanBackend()
{
    shutdown();
}

bool VulkanBackend::init(QWindow* window)
{
    if (!window) {
        qCWarning(lcVulkan) << "init() called with null window";
        return false;
    }
    m_impl->window = window;
    window->setSurfaceType(QSurface::VulkanSurface);

    // A QVulkanInstance must already be attached to the window by the caller
    // (application main or MonitorController).
    if (!window->vulkanInstance()) {
        qCWarning(lcVulkan) << "QVulkanInstance not set on window — cannot init Vulkan backend";
        return false;
    }

    QRhiVulkanInitParams params;
    params.inst = window->vulkanInstance();
    params.window = window;

    m_impl->rhi.reset(QRhi::create(QRhi::Vulkan, &params));
    if (!m_impl->rhi) {
        qCWarning(lcVulkan) << "QRhi::create(Vulkan) failed";
        return false;
    }
    qCInfo(lcVulkan) << "QRhi Vulkan initialised:"
                     << m_impl->rhi->driverInfo().deviceName;

    m_impl->swapChain.reset(m_impl->rhi->newSwapChain());
    m_impl->swapChain->setWindow(window);

    m_impl->rpDesc.reset(
        m_impl->swapChain->newCompatibleRenderPassDescriptor());
    m_impl->swapChain->setRenderPassDescriptor(m_impl->rpDesc.get());

    if (!m_impl->swapChain->createOrResize()) {
        qCWarning(lcVulkan) << "swapChain->createOrResize() failed";
        m_impl->destroyResources();
        return false;
    }

    // Shared VB: pos(x,y)+uv(u,v), 4 verts × 16 bytes = 64 bytes
    m_impl->quadVBuf.reset(m_impl->rhi->newBuffer(
        QRhiBuffer::Dynamic, QRhiBuffer::VertexBuffer, 64));
    if (!m_impl->quadVBuf->create()) {
        qCWarning(lcVulkan) << "VB create failed";
        m_impl->destroyResources();
        return false;
    }

    // Shared IB: 6 uint16 indices = 12 bytes
    m_impl->quadIBuf.reset(m_impl->rhi->newBuffer(
        QRhiBuffer::Immutable, QRhiBuffer::IndexBuffer, 12));
    if (!m_impl->quadIBuf->create()) {
        qCWarning(lcVulkan) << "IB create failed";
        m_impl->destroyResources();
        return false;
    }

    m_impl->initialised = true;
    qCInfo(lcVulkan) << "VulkanBackend fully initialised";
    return true;
}

void VulkanBackend::resizeSurface(const QSize& size)
{
    if (!m_impl->initialised) return;
    Q_UNUSED(size);
    if (!m_impl->swapChain->createOrResize()) {
        qCWarning(lcVulkan) << "swapChain->createOrResize() failed on resize";
    }
}

void VulkanBackend::beginFrame()
{
    if (!m_impl->initialised) return;
    const QRhi::FrameOpResult r = m_impl->rhi->beginFrame(m_impl->swapChain.get());
    if (r != QRhi::FrameOpSuccess) {
        qCWarning(lcVulkan) << "beginFrame() returned" << r;
        return;
    }
    m_impl->cb = m_impl->swapChain->currentFrameCommandBuffer();

    QRhiRenderTarget* rt = m_impl->swapChain->currentFrameRenderTarget();
    m_impl->cb->beginPass(rt, QColor(0, 0, 0, 255), {1.0f, 0});
}

void VulkanBackend::drawTile(TileId id, const DecodedFrameRef& frame, const QRectF& dst)
{
    if (!m_impl->initialised || !m_impl->cb) return;
    if (!frame.isValid()) return;
    Q_UNUSED(id);

    // VA-API/Vulkan surface import seam: frame.payload will be a
    // VkImage handle wrapped in a QRhiTexture::createFrom(NativeTexture).
    // For now, compute NDC quad coordinates for instrumentation.
    const QSize wsize = m_impl->swapChain->currentPixelSize();
    const float sx = static_cast<float>(dst.x() / wsize.width())  * 2.0f - 1.0f;
    const float sy = 1.0f - static_cast<float>(dst.y() / wsize.height()) * 2.0f;
    const float ex = static_cast<float>(dst.right()  / wsize.width())  * 2.0f - 1.0f;
    const float ey = 1.0f - static_cast<float>(dst.bottom() / wsize.height()) * 2.0f;

    const float verts[16] = {
        sx, ey, 0.0f, 1.0f,
        ex, ey, 1.0f, 1.0f,
        ex, sy, 1.0f, 0.0f,
        sx, sy, 0.0f, 0.0f,
    };
    Q_UNUSED(verts);
}

void VulkanBackend::endFrame()
{
    if (!m_impl->initialised || !m_impl->cb) return;
    m_impl->cb->endPass();
}

void VulkanBackend::submitPresent()
{
    if (!m_impl->initialised) return;
    m_impl->rhi->endFrame(m_impl->swapChain.get());
}

void VulkanBackend::shutdown()
{
    if (!m_impl->initialised) return;
    m_impl->destroyResources();
    qCInfo(lcVulkan) << "VulkanBackend shut down";
}

} // namespace Kaivue::Render

#endif // Q_OS_LINUX
