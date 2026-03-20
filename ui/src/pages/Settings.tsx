import { useState, useEffect } from 'react'
import { apiFetch } from '../api/client'

export default function Settings() {
  const [systemInfo, setSystemInfo] = useState<any>(null)

  useEffect(() => {
    apiFetch('/system/info').then(async res => {
      if (res.ok) setSystemInfo(await res.json())
    })
  }, [])

  return (
    <div>
      <h1 className="text-2xl font-bold text-nvr-text-primary mb-6">Settings</h1>

      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-5">
        <h2 className="text-lg font-semibold text-nvr-text-primary mb-4">System Information</h2>
        {systemInfo ? (
          <div>
            <div className="flex justify-between py-3 border-b border-nvr-border/50">
              <span className="text-sm text-nvr-text-secondary">Version</span>
              <span className="text-sm text-nvr-text-primary">{systemInfo.version}</span>
            </div>
            <div className="flex justify-between py-3 border-b border-nvr-border/50">
              <span className="text-sm text-nvr-text-secondary">Platform</span>
              <span className="text-sm text-nvr-text-primary">{systemInfo.platform}</span>
            </div>
            <div className="flex justify-between py-3">
              <span className="text-sm text-nvr-text-secondary">Uptime</span>
              <span className="text-sm text-nvr-text-primary">{systemInfo.uptime}</span>
            </div>
          </div>
        ) : (
          <p className="text-nvr-text-muted text-sm">Loading...</p>
        )}
      </div>
    </div>
  )
}
