import { useState, useEffect } from 'react'
import { useCameras } from '../hooks/useCameras'
import { useTimeline } from '../hooks/useRecordings'
import Timeline from '../components/Timeline'

export default function Recordings() {
  const { cameras, loading: camerasLoading } = useCameras()
  const [selectedCamera, setSelectedCamera] = useState<string | null>(null)
  const [date, setDate] = useState(new Date().toISOString().split('T')[0])
  const { ranges, loading: timelineLoading, load } = useTimeline(selectedCamera, date)

  useEffect(() => {
    if (selectedCamera && date) load()
  }, [selectedCamera, date])

  if (camerasLoading) return <div>Loading...</div>

  return (
    <div>
      <h1>Recordings</h1>

      <div style={{ marginBottom: 16 }}>
        <select value={selectedCamera || ''} onChange={e => setSelectedCamera(e.target.value || null)}>
          <option value="">Select Camera</option>
          {cameras.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
        </select>
        <input type="date" value={date} onChange={e => setDate(e.target.value)} style={{ marginLeft: 8 }} />
      </div>

      {selectedCamera && (
        <>
          <Timeline ranges={ranges} date={date} onSeek={(time) => {
            console.log('Seek to:', time)
          }} />
          {timelineLoading && <p>Loading timeline...</p>}
        </>
      )}
    </div>
  )
}
