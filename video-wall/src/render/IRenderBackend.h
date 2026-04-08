#pragma once

#include "DecodedFrame.h"

#include <QSize>
#include <QRectF>
#include <QWindow>

namespace Kaivue::Render {

/**
 * Platform-abstract render interface.
 *
 * Implementations:
 *   D3D12Backend   — Windows, Qt RHI D3D12   (requires Qt 6.6+)
 *   VulkanBackend  — Linux,   Qt RHI Vulkan  (requires Qt 6.6+)
 *   MetalBackend   — macOS,   Qt RHI Metal   (stub, not required for production)
 *   NullDecoderBackend — CPU software rasterizer, used by tests + CI
 *
 * Threading: all methods must be called from the render thread.
 * init() and shutdown() are called once per surface lifetime.
 */
class IRenderBackend {
public:
    virtual ~IRenderBackend() = default;

    /**
     * Initialise the backend against the given window.
     * @return true on success; false if the platform is unsupported.
     */
    virtual bool init(QWindow* window) = 0;

    /**
     * Called when the window surface is resized (including initial creation).
     */
    virtual void resizeSurface(const QSize& size) = 0;

    /**
     * Begin a new frame.  Must be called before any drawTile().
     */
    virtual void beginFrame() = 0;

    /**
     * Blit a decoded frame into the tile destination rectangle.
     * @param id     Tile being drawn.
     * @param frame  Decoded frame reference — do not retain past endFrame().
     * @param dst    Normalised destination rectangle in window coordinates
     *               (0,0)–(width,height).
     */
    virtual void drawTile(TileId id, const DecodedFrameRef& frame, const QRectF& dst) = 0;

    /**
     * End the frame — finalise GPU command lists.
     */
    virtual void endFrame() = 0;

    /**
     * Present the completed frame to the display.
     * May block up to one vsync interval.
     */
    virtual void submitPresent() = 0;

    /**
     * Tear down GPU resources.  Safe to call multiple times.
     */
    virtual void shutdown() = 0;
};

} // namespace Kaivue::Render
