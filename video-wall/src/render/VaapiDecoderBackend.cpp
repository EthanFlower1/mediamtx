#include "VaapiDecoderBackend.h"

#if defined(KAIVUE_ENABLE_VAAPI)

#include <QLoggingCategory>

extern "C" {
#include <libavcodec/avcodec.h>
#include <libavutil/hwcontext.h>
#include <libavutil/hwcontext_vaapi.h>
#include <libavutil/pixfmt.h>
}

Q_LOGGING_CATEGORY(lcVaapi, "kaivue.render.vaapi")

namespace Kaivue::Render {

namespace {
// hwaccel get_format callback — prefer VA-API surfaces.
AVPixelFormat vaapiGetFormat(AVCodecContext* /*ctx*/, const AVPixelFormat* fmts)
{
    for (const AVPixelFormat* p = fmts; *p != AV_PIX_FMT_NONE; ++p) {
        if (*p == AV_PIX_FMT_VAAPI) return *p;
    }
    qCWarning(lcVaapi) << "VA-API surface format not offered by decoder";
    return AV_PIX_FMT_NONE;
}
} // namespace

VaapiDecoderBackend::VaapiDecoderBackend() = default;
VaapiDecoderBackend::~VaapiDecoderBackend() { shutdown(); }

bool VaapiDecoderBackend::init(QWindow* /*window*/)
{
    if (m_initialised) return true;

    const int rc = av_hwdevice_ctx_create(&m_hwDeviceCtx,
                                          AV_HWDEVICE_TYPE_VAAPI,
                                          /*device=*/nullptr,
                                          /*opts=*/nullptr,
                                          /*flags=*/0);
    if (rc < 0) {
        qCWarning(lcVaapi) << "av_hwdevice_ctx_create(VAAPI) failed:" << rc;
        return false;
    }
    qCInfo(lcVaapi) << "VA-API hw device context created";
    m_initialised = true;
    return true;
}

void VaapiDecoderBackend::resizeSurface(const QSize& size)
{
    m_surfaceSize = size.isValid() ? size : m_surfaceSize;
}

void VaapiDecoderBackend::beginFrame() {}
void VaapiDecoderBackend::drawTile(TileId, const DecodedFrameRef&, const QRectF&) {}
void VaapiDecoderBackend::endFrame() {}
void VaapiDecoderBackend::submitPresent() {}

void VaapiDecoderBackend::shutdown()
{
    closeDecoder();
    if (m_hwDeviceCtx) {
        av_buffer_unref(&m_hwDeviceCtx);
    }
    m_initialised = false;
}

bool VaapiDecoderBackend::openDecoder(int codecId)
{
    const AVCodec* codec = avcodec_find_decoder(static_cast<AVCodecID>(codecId));
    if (!codec) {
        qCWarning(lcVaapi) << "avcodec_find_decoder failed for codec id" << codecId;
        return false;
    }

    m_codecCtx = avcodec_alloc_context3(codec);
    if (!m_codecCtx) return false;

    m_codecCtx->hw_device_ctx = av_buffer_ref(m_hwDeviceCtx);
    m_codecCtx->get_format    = vaapiGetFormat;

    if (avcodec_open2(m_codecCtx, codec, nullptr) < 0) {
        qCWarning(lcVaapi) << "avcodec_open2 failed";
        closeDecoder();
        return false;
    }
    return true;
}

void VaapiDecoderBackend::closeDecoder()
{
    if (m_codecCtx) {
        avcodec_free_context(&m_codecCtx);
        m_codecCtx = nullptr;
    }
}

bool VaapiDecoderBackend::submitPacket(AVPacket* packet)
{
    if (!m_codecCtx) return false;
    return avcodec_send_packet(m_codecCtx, packet) >= 0;
}

AVFrame* VaapiDecoderBackend::receiveFrame()
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

#endif // KAIVUE_ENABLE_VAAPI
