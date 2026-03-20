import { Camera } from '../hooks/useCameras'
import PlayerCell from './PlayerCell'

interface Props {
  cameras: Camera[]
  layout: number
  onSelectCamera?: (camera: Camera) => void
}

const gridColsMap: Record<number, string> = {
  1: 'grid-cols-1',
  2: 'grid-cols-2',
  3: 'grid-cols-3',
  4: 'grid-cols-4',
}

export default function CameraGrid({ cameras, layout, onSelectCamera }: Props) {
  return (
    <div className={`grid ${gridColsMap[layout] ?? 'grid-cols-2'} gap-1 w-full`}>
      {cameras.map(cam => (
        <PlayerCell
          key={cam.id}
          camera={cam}
          onSelect={() => onSelectCamera?.(cam)}
        />
      ))}
    </div>
  )
}
