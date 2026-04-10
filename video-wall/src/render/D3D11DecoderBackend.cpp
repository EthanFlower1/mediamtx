#include "D3D11DecoderBackend.h"

#if defined(KAIVUE_ENABLE_D3D11)

#include <QLoggingCategory>

extern "C" {
#include <libavcodec/avcodec.h>
#include <libavutil/hwcontext.h>
#include <libavutil/hwcontext_d3d11va.h>
#include <libavutil/pixfmt.h>
}

Q_LOGGING_CATEGORY(lcD3D11, "kaivue.render.d3d11")

namespace Kaivue::Render {

namespace {
AVPixelFormat d3d11GetFormat(AVCodecContext* /*ctx*/, const AVPixelFormat* fmts)
{
    for (const AVPixelFormat* p = fmts; *p != AV_PIX_FMT_NONE; ++p) {
        if (*p == AV_PIX_FMT_D3D11) return *p;
    }
    qCWarning(lcD3D11) << "D3D11 surface format not offered by decoder";
    return AV_PIX_FMT_NONE;
}
} // namespace

D3D11DecoderBackend::D3D11DecoderBackend() = default;
D3D11DecoderBackend::~D3D11DecoderBackend() { shutdown(); }

bool D3D11DecoderBackend::init(QWindow* /*window*/)
{
    if (m_initialised) return true;

    const int rc = av_hwdevice_ctx_create(&m_hwDeviceCtx,
                                          AV_HWDEVICE_TYPE_D3D11VA,
                                          nullptr, nullptr, 0);
    if (rc < 0) {
        qCWarning(lcD3D11) << "av_hwdevice_ctx_create(D3D11VA) failed:" << rc;
        return false;
    }
    qCInfo(lcD3D11) << "D3D11VA hw device context created";
    m_initialised = true;
    return true;
}

void D3D11DecoderBackend::resizeSurface(const QSize& size)
{
    m_surfaceSize = size.isValid() ? size : m_surfaceSize;
}

void D3D11DecoderBackend::beginFrame() {}
void D3D11DecoderBackend::drawTile(TileId, const DecodedFrameRef&, const QRectF&) {}
void D3D11DecoderBackend::endFrame() {}
void D3D11DecoderBackend::submitPresent() {}

void D3D11DecoderBackend::shutdown()
{
    closeDecoder();
    if (m_hwDeviceCtx) {
        av_buffer_unref(&m_hwDeviceCtx);
    }
    m_initialised = false;
}

bool D3D11DecoderBackend::openDecoder(int codecId)
{
    const AVCodec* codec = avcodec_find_decoder(static_cast<AVCodecID>(codecId));
    if (!codec) return false;

    m_codecCtx = avcodec_alloc_context3(codec);
    if (!m_codecCtx) return false;

    m_codecCtx->hw_device_ctx = av_buffer_ref(m_hwDeviceCtx);
    m_codecCtx->get_format    = d3d11GetFormat;

    if (avcodec_open2(m_codecCtx, codec, nullptr) < 0) {
        closeDecoder();
        return false;
    }
    return true;
}

void D3D11DecoderBackend::closeDecoder()
{
    if (m_codecCtx) {
        avcodec_free_context(&m_codecCtx);
        m_codecCtx = nullptr;
    }
}

bool D3D11DecoderBackend::submitPacket(AVPacket* packet)
{
    if (!m_codecCtx) return false;
    return avcodec_send_packet(m_codecCtx, packet) >= 0;
}

AVFrame* D3D11DecoderBackend::receiveFrame()
{
    if (!m_codecCtx) return nullptr;
    AVFrame* frame = av_frame_alloc();
    if (!frame) return nullptr;
    if (avcodec_receive_frame(m_codecCtx, frame) < 0) {
        av_frame_free(&frame);
        return nullptr;
    }
    return frame;
}

} // namespace Kaivue::Render

#endif // KAIVUE_ENABLE_D3D11
