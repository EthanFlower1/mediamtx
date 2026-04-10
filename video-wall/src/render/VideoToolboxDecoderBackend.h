#pragma once

//
// VideoToolboxDecoderBackend — macOS hardware-accelerated H.264/H.265 decode
// via Apple VideoToolbox, driven through ffmpeg's hwaccel API.
//
// SCAFFOLD added by KAI-333. See VaapiDecoderBackend.h header comment for
// the overall pattern. Guarded by KAIVUE_ENABLE_VIDEOTOOLBOX.
//

#if defined(KAIVUE_ENABLE_VIDEOTOOLBOX)

#include "IRenderBackend.h"

extern "C" {
struct AVBufferRef;
struct AVCodecContext;
struct AVFrame;
struct AVPacket;
}

namespace Kaivue::Render {

class VideoToolboxDecoderBackend : public IRenderBackend {
    Q_DISABLE_COPY_MOVE(VideoToolboxDecoderBackend)
public:
    VideoToolboxDecoderBackend();
    ~VideoToolboxDecoderBackend() override;

    bool init(QWindow* window) override;
    void resizeSurface(const QSize& size) override;
    void beginFrame() override;
    void drawTile(TileId id, const DecodedFrameRef& frame, const QRectF& dst) override;
    void endFrame() override;
    void submitPresent() override;
    void shutdown() override;

    bool     submitPacket(AVPacket* packet);
    AVFrame* receiveFrame();

private:
    bool openDecoder(int codecId);
    void closeDecoder();

    AVBufferRef*    m_hwDeviceCtx{nullptr};
    AVCodecContext* m_codecCtx{nullptr};
    QSize           m_surfaceSize{1920, 1080};
    bool            m_initialised{false};
};

} // namespace Kaivue::Render

#endif // KAIVUE_ENABLE_VIDEOTOOLBOX
