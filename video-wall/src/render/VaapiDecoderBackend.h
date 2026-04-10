#pragma once

//
// VaapiDecoderBackend — Linux hardware-accelerated H.264/H.265 decode via
// libva (VA-API), driven through ffmpeg's hwaccel API.
//
// This is a SCAFFOLD added by KAI-333. It implements the IRenderBackend
// contract so the video wall can slot it in, but the decode path only
// performs AVCodecContext / AVHWDeviceContext setup — no tuning, no
// zero-copy GPU blit yet. Full wiring happens in follow-up tickets.
//
// Compile-time guard: the implementation is compiled only when
// KAIVUE_ENABLE_VAAPI is defined. Otherwise the .cpp file is an empty
// translation unit, keeping CI (which targets NullDecoderBackend) free
// of any ffmpeg/libva dependency.
//

#if defined(KAIVUE_ENABLE_VAAPI)

#include "IRenderBackend.h"
#include <memory>

extern "C" {
struct AVBufferRef;
struct AVCodecContext;
struct AVFrame;
struct AVPacket;
}

namespace Kaivue::Render {

class VaapiDecoderBackend : public IRenderBackend {
    Q_DISABLE_COPY_MOVE(VaapiDecoderBackend)
public:
    VaapiDecoderBackend();
    ~VaapiDecoderBackend() override;

    bool init(QWindow* window) override;
    void resizeSurface(const QSize& size) override;
    void beginFrame() override;
    void drawTile(TileId id, const DecodedFrameRef& frame, const QRectF& dst) override;
    void endFrame() override;
    void submitPresent() override;
    void shutdown() override;

    /// Feed a compressed AVPacket into the decoder. Returns true if the
    /// packet was accepted (not necessarily fully consumed).
    bool submitPacket(AVPacket* packet);

    /// Pull one decoded frame (hw surface). Returns nullptr if none ready.
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

#endif // KAIVUE_ENABLE_VAAPI
