import { useState, useEffect, useCallback } from 'react'
import { apiFetch } from '../api/client'

interface RelayOutput {
  token: string
  mode: string
  idle_state: string
}

interface Props {
  cameraId: string
}

export default function RelayControls({ cameraId }: Props) {
  const [outputs, setOutputs] = useState<RelayOutput[]>([])
  const [activeStates, setActiveStates] = useState<Record<string, boolean>>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [toggling, setToggling] = useState<string | null>(null)

  useEffect(() => {
    setLoading(true)
    setError(null)
    apiFetch(`/cameras/${cameraId}/relay-outputs`)
      .then(res => {
        if (res.status === 501) {
          setOutputs([])
          return null
        }
        if (!res.ok) throw new Error('Failed to fetch relay outputs')
        return res.json()
      })
      .then(data => {
        if (data?.relay_outputs) {
          setOutputs(data.relay_outputs)
          const states: Record<string, boolean> = {}
          for (const o of data.relay_outputs) {
            states[o.token] = false
          }
          setActiveStates(states)
        }
      })
      .catch(() => setError('Failed to load relay outputs'))
      .finally(() => setLoading(false))
  }, [cameraId])

  const toggleRelay = useCallback((token: string) => {
    const newState = !activeStates[token]
    setToggling(token)
    apiFetch(`/cameras/${cameraId}/relay-outputs/${token}/state`, {
      method: 'POST',
      body: JSON.stringify({ active: newState }),
    })
      .then(res => {
        if (res.ok) {
          setActiveStates(prev => ({ ...prev, [token]: newState }))
        }
      })
      .catch(() => {})
      .finally(() => setToggling(null))
  }, [cameraId, activeStates])

  if (loading) {
    return (
      <div className="bg-nvr-bg-secondary rounded-lg p-3">
        <div className="text-xs text-nvr-text-muted animate-pulse">Loading relay outputs...</div>
      </div>
    )
  }

  if (error || outputs.length === 0) {
    return (
      <div className="bg-nvr-bg-secondary rounded-lg p-3">
        <div className="text-xs text-nvr-text-muted">No relay outputs available</div>
      </div>
    )
  }

  return (
    <div className="bg-nvr-bg-secondary rounded-lg p-3">
      <h4 className="text-xs font-semibold text-nvr-text-secondary uppercase tracking-wider mb-2">
        Relay Outputs
      </h4>
      <div className="space-y-2">
        {outputs.map(output => (
          <div key={output.token} className="flex items-center justify-between">
            <div className="flex flex-col">
              <span className="text-sm text-nvr-text-primary font-medium">
                {output.token}
              </span>
              <span className="text-xs text-nvr-text-muted">
                Mode: {output.mode || 'N/A'} | Idle: {output.idle_state || 'N/A'}
              </span>
            </div>
            <button
              onClick={() => toggleRelay(output.token)}
              disabled={toggling === output.token}
              className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
                activeStates[output.token]
                  ? 'bg-nvr-accent'
                  : 'bg-nvr-bg-tertiary border border-nvr-border'
              } ${toggling === output.token ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}`}
              aria-label={`Toggle relay ${output.token}`}
            >
              <span
                className={`inline-block h-4 w-4 rounded-full bg-white shadow-sm transform transition-transform ${
                  activeStates[output.token] ? 'translate-x-6' : 'translate-x-1'
                }`}
              />
            </button>
          </div>
        ))}
      </div>
    </div>
  )
}
