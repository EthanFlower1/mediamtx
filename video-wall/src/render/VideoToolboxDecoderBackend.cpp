#include "VideoToolboxDecoderBackend.h"

#if defined(KAIVUE_ENABLE_VIDEOTOOLBOX)

#include <QLoggingCategory>

extern "C" {
#include <libavcodec/avcodec.h>
#include <libavutil/hwcontext.h>
#include <libavutil/pixfmt.h>
}

Q_LOGGING_CATEGORY(lcVT, "kaivue.render.videotoolbox")

namespace Kaivue::Render {

namespace {
AVPixelFormat vtGetFormat(AVCodecContext* /*ctx*/, const AVPixelFormat* fmts)
{
    for (const AVPixelFormat* p = fmts; *p != AV_PIX_FMT_NONE; ++p) {
        if (*p == AV_PIX_FMT_VIDEOTOOLBOX) return *p;
    }
    qCWarning(lcVT) << "VideoToolbox surface format not offered by decoder";
    return AV_PIX_FMT_NONE;
}
} // namespace

VideoToolboxDecoderBackend::VideoToolboxDecoderBackend() = default;
VideoToolboxDecoderBackend::~VideoToolboxDecoderBackend() { shutdown(); }

bool VideoToolboxDecoderBackend::init(QWindow* /*window*/)
{
    if (m_initialised) return true;

    const int rc = av_hwdevice_ctx_create(&m_hwDeviceCtx,
                                          AV_HWDEVICE_TYPE_VIDEOTOOLBOX,
                                          nullptr, nullptr, 0);
    if (rc < 0) {
        qCWarning(lcVT) << "av_hwdevice_ctx_create(VIDEOTOOLBOX) failed:" << rc;
        return false;
    }
    qCInfo(lcVT) << "VideoToolbox hw device context created";
    m_initialised = true;
    return true;
}

void VideoToolboxDecoderBackend::resizeSurface(const QSize& size)
{
    m_surfaceSize = size.isValid() ? size : m_surfaceSize;
}

void VideoToolboxDecoderBackend::beginFrame() {}
void VideoToolboxDecoderBackend::drawTile(TileId, const DecodedFrameRef&, const QRectF&) {}
void VideoToolboxDecoderBackend::endFrame() {}
void VideoToolboxDecoderBackend::submitPresent() {}

void VideoToolboxDecoderBackend::shutdown()
{
    closeDecoder();
    if (m_hwDeviceCtx) {
        av_buffer_unref(&m_hwDeviceCtx);
    }
    m_initialised = false;
}

bool VideoToolboxDecoderBackend::openDecoder(int codecId)
{
    const AVCodec* codec = avcodec_find_decoder(static_cast<AVCodecID>(codecId));
    if (!codec) return false;

    m_codecCtx = avcodec_alloc_context3(codec);
    if (!m_codecCtx) return false;

    m_codecCtx->hw_device_ctx = av_buffer_ref(m_hwDeviceCtx);
    m_codecCtx->get_format    = vtGetFormat;

    if (avcodec_open2(m_codecCtx, codec, nullptr) < 0) {
        closeDecoder();
        return false;
    }
    return true;
}

void VideoToolboxDecoderBackend::closeDecoder()
{
    if (m_codecCtx) {
        avcodec_free_context(&m_codecCtx);
        m_codecCtx = nullptr;
    }
}

bool VideoToolboxDecoderBackend::submitPacket(AVPacket* packet)
{
    if (!m_codecCtx) return false;
    return avcodec_send_packet(m_codecCtx, packet) >= 0;
}

AVFrame* VideoToolboxDecoderBackend::receiveFrame()
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

#endif // KAIVUE_ENABLE_VIDEOTOOLBOX
