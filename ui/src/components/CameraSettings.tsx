import { useState, useEffect } from 'react'
import { apiFetch } from '../api/client'

interface ImagingSettings {
  brightness: number
  contrast: number
  saturation: number
  sharpness: number
}

interface CameraSettingsProps {
  cameraId: string
  onClose: () => void
}

export default function CameraSettings({ cameraId, onClose }: CameraSettingsProps) {
  const [settings, setSettings] = useState<ImagingSettings | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')

  useEffect(() => {
    setLoading(true)
    setError('')
    apiFetch(`/cameras/${cameraId}/settings`)
      .then(async res => {
        if (res.ok) {
          setSettings(await res.json())
        } else {
          const data = await res.json().catch(() => ({}))
          setError(data.error || 'Failed to load settings')
        }
      })
      .catch(() => setError('Network error'))
      .finally(() => setLoading(false))
  }, [cameraId])

  const handleChange = (key: keyof ImagingSettings, value: number) => {
    if (!settings) return
    setSettings({ ...settings, [key]: value })
    setSuccess('')
  }

  const handleApply = async () => {
    if (!settings) return
    setSaving(true)
    setError('')
    setSuccess('')

    try {
      const res = await apiFetch(`/cameras/${cameraId}/settings`, {
        method: 'PUT',
        body: JSON.stringify(settings),
      })
      if (res.ok) {
        setSuccess('Settings applied successfully')
      } else {
        const data = await res.json().catch(() => ({}))
        setError(data.error || 'Failed to apply settings')
      }
    } catch {
      setError('Network error')
    } finally {
      setSaving(false)
    }
  }

  const handleReset = () => {
    setSettings({
      brightness: 50,
      contrast: 50,
      saturation: 50,
      sharpness: 50,
    })
    setSuccess('')
  }

  if (loading) {
    return (
      <div className="p-4">
        <p className="text-nvr-text-muted text-sm">Loading camera settings...</p>
      </div>
    )
  }

  if (error && !settings) {
    return (
      <div className="p-4">
        <p className="text-nvr-danger text-sm">{error}</p>
        <button
          onClick={onClose}
          className="mt-2 text-sm text-nvr-text-muted hover:text-nvr-text-secondary transition-colors"
        >
          Close
        </button>
      </div>
    )
  }

  if (!settings) return null

  const sliders: { key: keyof ImagingSettings; label: string }[] = [
    { key: 'brightness', label: 'Brightness' },
    { key: 'contrast', label: 'Contrast' },
    { key: 'saturation', label: 'Saturation' },
    { key: 'sharpness', label: 'Sharpness' },
  ]

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-nvr-text-primary">Image Settings</h3>
        <button
          onClick={onClose}
          className="text-nvr-text-muted hover:text-nvr-text-secondary text-lg bg-transparent border-none cursor-pointer min-w-[44px] min-h-[44px] flex items-center justify-center"
        >
          &times;
        </button>
      </div>

      {sliders.map(({ key, label }) => (
        <div key={key}>
          <div className="flex justify-between mb-1">
            <label className="text-xs text-nvr-text-secondary">{label}</label>
            <span className="text-xs text-nvr-text-muted font-mono">{Math.round(settings[key])}</span>
          </div>
          <input
            type="range"
            min={0}
            max={100}
            value={settings[key]}
            onChange={e => handleChange(key, parseFloat(e.target.value))}
            className="w-full h-2 bg-nvr-bg-primary rounded-lg appearance-none cursor-pointer accent-nvr-accent"
          />
        </div>
      ))}

      {error && <p className="text-nvr-danger text-xs">{error}</p>}
      {success && <p className="text-nvr-success text-xs">{success}</p>}

      <div className="flex gap-2 pt-1">
        <button
          onClick={handleApply}
          disabled={saving}
          className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-3 py-1.5 rounded-lg transition-colors disabled:opacity-50 text-sm min-h-[44px]"
        >
          {saving ? 'Applying...' : 'Apply'}
        </button>
        <button
          onClick={handleReset}
          className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-3 py-1.5 rounded-lg border border-nvr-border transition-colors text-sm min-h-[44px]"
        >
          Reset to Default
        </button>
      </div>
    </div>
  )
}
