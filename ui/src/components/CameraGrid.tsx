import { Camera } from '../hooks/useCameras'
import PlayerCell from './PlayerCell'

interface Props {
  cameras: Camera[]
  layout: number
  onSelectCamera?: (camera: Camera) => void
}

export default function CameraGrid({ cameras, layout, onSelectCamera }: Props) {
  return (
    <div style={{
      display: 'grid',
      gridTemplateColumns: `repeat(${layout}, 1fr)`,
      gap: 4,
      width: '100%',
    }}>
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
