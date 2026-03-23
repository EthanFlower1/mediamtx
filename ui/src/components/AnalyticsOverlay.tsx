import { useRef, useEffect, useState, useCallback } from 'react'
import { apiFetch } from '../api/client'

export interface Detection {
  id: number
  class: string
  confidence: number
  box_x: number
  box_y: number
  box_w: number
  box_h: number
  frame_time: string
  track_id?: number
}

interface Props {
  cameraId: string
  videoElement: HTMLVideoElement | null
  enabled: boolean
}

/** Color for a given COCO class name. */
function classColor(className: string): string {
  const lower = className.toLowerCase()
  if (lower === 'person') return '#3b82f6' // blue-500
  // Vehicles
  if (['car', 'truck', 'bus', 'motorcycle', 'bicycle', 'boat', 'airplane', 'train'].includes(lower))
    return '#22c55e' // green-500
  // Animals
  if (['cat', 'dog', 'horse', 'sheep', 'cow', 'elephant', 'bear', 'zebra', 'giraffe', 'bird'].includes(lower))
    return '#f59e0b' // amber-500
  return '#ef4444' // red-500
}

/** Map COCO class to a display-friendly category label. */
function displayLabel(className: string): string {
  const lower = className.toLowerCase()
  if (['car', 'truck', 'bus', 'motorcycle', 'bicycle', 'boat', 'airplane', 'train'].includes(lower))
    return 'Vehicle'
  if (['cat', 'dog', 'horse', 'sheep', 'cow', 'elephant', 'bear', 'zebra', 'giraffe', 'bird'].includes(lower))
    return 'Animal'
  return className.charAt(0).toUpperCase() + className.slice(1)
}

/** Format a class label with confidence, e.g. "Person 94%" */
function formatLabel(className: string, confidence: number): string {
  return `${displayLabel(className)} ${Math.round(confidence * 100)}%`
}

/**
 * AnalyticsOverlay renders a `<canvas>` positioned over a video element
 * to draw bounding boxes for detected objects. It polls the NVR API every
 * 500ms for the latest detections and draws them with fade-out.
 */
export default function AnalyticsOverlay({ cameraId, videoElement, enabled }: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const [detections, setDetections] = useState<Detection[]>([])
  const [zones, setZones] = useState<Array<{id: number, name: string, polygon: [number, number][]}>>([])
  const animFrameRef = useRef<number>(0)
  const lastUpdateRef = useRef<number>(Date.now())

  // Poll the API every 500ms for new detections.
  useEffect(() => {
    if (!enabled || !cameraId) return

    let cancelled = false

    const poll = async () => {
      try {
        const resp = await apiFetch(`/cameras/${cameraId}/detections/latest`)
        if (resp.ok && !cancelled) {
          const data: Detection[] = await resp.json()
          setDetections(data)
          lastUpdateRef.current = Date.now()
        }
      } catch {
        // Silently ignore fetch errors — will retry next interval.
      }
    }

    poll()
    const interval = setInterval(poll, 500)
    return () => {
      cancelled = true
      clearInterval(interval)
    }
  }, [enabled, cameraId])

  // Poll zones every 30 seconds (zones change rarely).
  useEffect(() => {
    if (!enabled || !cameraId) return
    let cancelled = false
    const fetchZones = async () => {
      try {
        const resp = await apiFetch(`/cameras/${cameraId}/zones`)
        if (resp.ok && !cancelled) {
          const data = await resp.json()
          setZones(data)
        }
      } catch { /* ignore */ }
    }
    fetchZones()
    const interval = setInterval(fetchZones, 30000)
    return () => { cancelled = true; clearInterval(interval) }
  }, [enabled, cameraId])

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

  /** Draw all detections onto the canvas with fade-out. */
  const draw = useCallback(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    ctx.clearRect(0, 0, canvas.width, canvas.height)

    // Draw zone polygons (always visible, no fade)
    for (const zone of zones) {
      if (!zone.polygon || zone.polygon.length < 3) continue
      ctx.beginPath()
      ctx.moveTo(zone.polygon[0][0] * canvas.width, zone.polygon[0][1] * canvas.height)
      for (let i = 1; i < zone.polygon.length; i++) {
        ctx.lineTo(zone.polygon[i][0] * canvas.width, zone.polygon[i][1] * canvas.height)
      }
      ctx.closePath()
      ctx.strokeStyle = '#6366f1' // indigo
      ctx.lineWidth = 1.5
      ctx.setLineDash([4, 4])
      ctx.stroke()
      ctx.setLineDash([])
      ctx.fillStyle = 'rgba(99, 102, 241, 0.08)' // very subtle fill
      ctx.fill()

      // Zone label
      ctx.font = '11px Inter, system-ui, sans-serif'
      ctx.fillStyle = 'rgba(99, 102, 241, 0.7)'
      ctx.fillText(zone.name, zone.polygon[0][0] * canvas.width + 4, zone.polygon[0][1] * canvas.height + 12)
    }

    // Fade out detections after 1 second since last update.
    const elapsed = Date.now() - lastUpdateRef.current
    const opacity = Math.max(0, 1 - (elapsed - 1000) / 500)
    if (opacity <= 0 || detections.length === 0) return

    ctx.globalAlpha = opacity

    for (const det of detections) {
      const color = classColor(det.class)
      // YOLO normalized coordinates: x,y are top-left; w,h are width/height (0..1)
      const x = det.box_x * canvas.width
      const y = det.box_y * canvas.height
      const w = det.box_w * canvas.width
      const h = det.box_h * canvas.height

      // Bounding box
      ctx.strokeStyle = color
      ctx.lineWidth = 2
      ctx.strokeRect(x, y, w, h)

      // Label background
      const label = det.track_id
        ? `${displayLabel(det.class)} #${det.track_id} ${Math.round(det.confidence * 100)}%`
        : formatLabel(det.class, det.confidence)
      ctx.font = '12px Inter, system-ui, sans-serif'
      const textMetrics = ctx.measureText(label)
      const textH = 16
      ctx.fillStyle = color
      ctx.fillRect(x, y - textH, textMetrics.width + 8, textH)

      // Label text
      ctx.fillStyle = '#ffffff'
      ctx.fillText(label, x + 4, y - 4)
    }

    ctx.globalAlpha = 1
  }, [detections, zones])

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
          <span className="text-xs text-white/80">Analytics overlay — waiting for detections</span>
        </div>
      )}
    </div>
  )
}
