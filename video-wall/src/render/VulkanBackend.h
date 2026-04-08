#pragma once

#ifdef Q_OS_LINUX

#include "IRenderBackend.h"

#include <memory>

// Qt RHI private headers — requires Qt6::GuiPrivate
#include <rhi/qrhi.h>

namespace Kaivue::Render {

/**
 * Linux Vulkan render backend via Qt RHI.
 *
 * Requires Qt 6.6+ and Vulkan 1.2+ driver.
 * Links against Qt6::GuiPrivate for <rhi/qrhi.h>.
 *
 * Architecture mirrors D3D12Backend:
 *   - QRhi::Vulkan instance with a QRhiSwapChain per QWindow.
 *   - VA-API surfaces from Intel QuickSync (333c) will be imported via
 *     VkImage + VK_KHR_external_memory_fd.
 *   - AMD AMF (333d) surfaces imported via VK_KHR_external_memory similarly.
 *   - NullDecoder uploads CPU RGBA to a VkImage via staging buffer.
 *
 * A QVulkanInstance must be created by the application before calling init().
 * The instance is retrieved via QVulkanInstance::current() or passed via
 * QWindow::setVulkanInstance() which QRhi picks up automatically.
 */
class VulkanBackend : public IRenderBackend {
    Q_DISABLE_COPY_MOVE(VulkanBackend)
public:
    VulkanBackend();
    ~VulkanBackend() override;

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

#endif // Q_OS_LINUX
