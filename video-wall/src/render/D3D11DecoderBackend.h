#pragma once

//
// D3D11DecoderBackend — Windows hardware-accelerated H.264/H.265 decode via
// DXVA2 on Direct3D 11 textures, driven through ffmpeg's hwaccel API.
//
// SCAFFOLD added by KAI-333. Guarded by KAIVUE_ENABLE_D3D11.
//

#if defined(KAIVUE_ENABLE_D3D11)

#include "IRenderBackend.h"

extern "C" {
struct AVBufferRef;
struct AVCodecContext;
struct AVFrame;
struct AVPacket;
}

namespace Kaivue::Render {

class D3D11DecoderBackend : public IRenderBackend {
    Q_DISABLE_COPY_MOVE(D3D11DecoderBackend)
public:
    D3D11DecoderBackend();
    ~D3D11DecoderBackend() override;

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

#endif // KAIVUE_ENABLE_D3D11
