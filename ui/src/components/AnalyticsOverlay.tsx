import { useRef, useEffect, useState, useCallback } from 'react'

export interface Detection {
  objectId: string
  className: string
  confidence: number
  box: { left: number; top: number; right: number; bottom: number }
}

interface Props {
  cameraId: string
  videoElement: HTMLVideoElement | null
  enabled: boolean
}

/** Color for a given class name. */
function classColor(className: string): string {
  switch (className.toLowerCase()) {
    case 'person':  return '#3b82f6' // blue-500
    case 'vehicle': return '#22c55e' // green-500
    case 'animal':  return '#f59e0b' // amber-500
    default:        return '#ef4444' // red-500
  }
}

/** Format a class label with confidence, e.g. "Person 94%" */
function formatLabel(className: string, confidence: number): string {
  const name = className.charAt(0).toUpperCase() + className.slice(1)
  return `${name} ${Math.round(confidence * 100)}%`
}

/**
 * AnalyticsOverlay renders a `<canvas>` positioned over a video element
 * to draw bounding boxes for detected objects.
 *
 * For v1 this is a structural placeholder — most consumer cameras do not
 * stream ONVIF metadata, so the overlay shows a waiting state until real
 * detections are fed in.
 */
export default function AnalyticsOverlay({ cameraId: _cameraId, videoElement, enabled }: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const [detections] = useState<Detection[]>([])
  const animFrameRef = useRef<number>(0)

  /** Resize the canvas to match the video's rendered size. */
  const syncSize = useCallback(() => {
    const canvas = canvasRef.current
    if (!canvas || !videoElement) return
    const rect = videoElement.getBoundingClientRect()
    canvas.width = rect.width
    canvas.height = rect.height
    canvas.style.width = `${rect.width}px`
    canvas.style.height = `${rect.height}px`
  }, [videoElement])

  /** Draw all detections onto the canvas. */
  const draw = useCallback(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    ctx.clearRect(0, 0, canvas.width, canvas.height)

    if (detections.length === 0) return

    for (const det of detections) {
      const color = classColor(det.className)
      // ONVIF normalized coordinates: -1..1 for both axes, center-based
      // Convert to pixel coordinates
      const x = ((det.box.left + 1) / 2) * canvas.width
      const y = ((det.box.top + 1) / 2) * canvas.height
      const w = ((det.box.right - det.box.left) / 2) * canvas.width
      const h = ((det.box.bottom - det.box.top) / 2) * canvas.height

      // Bounding box
      ctx.strokeStyle = color
      ctx.lineWidth = 2
      ctx.strokeRect(x, y, w, h)

      // Label background
      const label = formatLabel(det.className, det.confidence)
      ctx.font = '12px Inter, system-ui, sans-serif'
      const textMetrics = ctx.measureText(label)
      const textH = 16
      ctx.fillStyle = color
      ctx.fillRect(x, y - textH, textMetrics.width + 8, textH)

      // Label text
      ctx.fillStyle = '#ffffff'
      ctx.fillText(label, x + 4, y - 4)
    }
  }, [detections])

  // Render loop: sync size and draw every frame while enabled
  useEffect(() => {
    if (!enabled || !videoElement) return

    const loop = () => {
      syncSize()
      draw()
      animFrameRef.current = requestAnimationFrame(loop)
    }
    animFrameRef.current = requestAnimationFrame(loop)

    return () => cancelAnimationFrame(animFrameRef.current)
  }, [enabled, videoElement, syncSize, draw])

  if (!enabled) return null

  return (
    <div className="absolute inset-0 pointer-events-none z-10">
      <canvas ref={canvasRef} className="absolute top-0 left-0" />
      {detections.length === 0 && (
        <div className="absolute bottom-3 left-3 bg-black/60 backdrop-blur-sm rounded-md px-3 py-1.5 flex items-center gap-2">
          <span className="inline-block w-2 h-2 rounded-full bg-nvr-accent animate-pulse" />
          <span className="text-xs text-white/80">Analytics overlay — waiting for metadata</span>
        </div>
      )}
    </div>
  )
}
