import { useState, useRef, useEffect, useCallback } from 'react'
import { Camera } from '../hooks/useCameras'
import PlayerCell from './PlayerCell'

interface Props {
  cameras: Camera[]
  layout: number
  onSelectCamera?: (camera: Camera) => void
}

const STORAGE_KEY = 'nvr-camera-order'

const gridColsMap: Record<number, string> = {
  1: 'grid-cols-1',
  2: 'grid-cols-1 sm:grid-cols-2',
  3: 'grid-cols-1 sm:grid-cols-2 lg:grid-cols-3',
  4: 'grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4',
}

function loadOrder(): string[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    return raw ? JSON.parse(raw) : []
  } catch {
    return []
  }
}

function saveOrder(ids: string[]) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(ids))
}

function sortCameras(cameras: Camera[], order: string[]): Camera[] {
  if (order.length === 0) return cameras
  const indexMap = new Map(order.map((id, i) => [id, i]))
  return [...cameras].sort((a, b) => {
    const ai = indexMap.get(a.id) ?? Infinity
    const bi = indexMap.get(b.id) ?? Infinity
    return ai - bi
  })
}

export default function CameraGrid({ cameras, layout, onSelectCamera }: Props) {
  const [order, setOrder] = useState<string[]>(loadOrder)
  const [dragIdx, setDragIdx] = useState<number | null>(null)
  const [overIdx, setOverIdx] = useState<number | null>(null)
  const dragCounterRef = useRef(0)

  // Re-sort when cameras list or saved order changes
  const sorted = sortCameras(cameras, order)

  // Persist order whenever it changes
  useEffect(() => {
    if (order.length > 0) saveOrder(order)
  }, [order])

  const handleDragStart = useCallback((e: React.DragEvent, idx: number) => {
    setDragIdx(idx)
    e.dataTransfer.effectAllowed = 'move'
    // Slight delay for the ghost image to render properly
    ;(e.target as HTMLElement).style.opacity = '0.4'
  }, [])

  const handleDragEnd = useCallback((e: React.DragEvent) => {
    ;(e.target as HTMLElement).style.opacity = '1'
    setDragIdx(null)
    setOverIdx(null)
    dragCounterRef.current = 0
  }, [])

  const handleDragEnter = useCallback((idx: number) => {
    dragCounterRef.current++
    setOverIdx(idx)
  }, [])

  const handleDragLeave = useCallback(() => {
    dragCounterRef.current--
    if (dragCounterRef.current <= 0) {
      setOverIdx(null)
      dragCounterRef.current = 0
    }
  }, [])

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
  }, [])

  const handleDrop = useCallback((e: React.DragEvent, dropIdx: number) => {
    e.preventDefault()
    if (dragIdx === null || dragIdx === dropIdx) return

    const newSorted = [...sorted]
    const [moved] = newSorted.splice(dragIdx, 1)
    newSorted.splice(dropIdx, 0, moved)
    setOrder(newSorted.map(c => c.id))
    setDragIdx(null)
    setOverIdx(null)
    dragCounterRef.current = 0
  }, [dragIdx, sorted])

  return (
    <div className={`grid ${gridColsMap[layout] ?? 'grid-cols-1 sm:grid-cols-2'} gap-2 w-full`}>
      {sorted.map((cam, idx) => (
        <div
          key={cam.id}
          draggable
          onDragStart={(e) => handleDragStart(e, idx)}
          onDragEnd={handleDragEnd}
          onDragEnter={() => handleDragEnter(idx)}
          onDragLeave={handleDragLeave}
          onDragOver={handleDragOver}
          onDrop={(e) => handleDrop(e, idx)}
          className={`transition-all duration-150 rounded-lg ${
            overIdx === idx && dragIdx !== null && dragIdx !== idx
              ? 'ring-2 ring-nvr-accent ring-offset-2 ring-offset-nvr-bg-primary'
              : ''
          }`}
        >
          <PlayerCell
            camera={cam}
            onSelect={() => onSelectCamera?.(cam)}
          />
        </div>
      ))}
    </div>
  )
}
