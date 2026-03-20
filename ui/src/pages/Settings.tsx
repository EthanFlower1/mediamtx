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
      <h1>Settings</h1>

      <h2>System Information</h2>
      {systemInfo ? (
        <table>
          <tbody>
            <tr><td>Version</td><td>{systemInfo.version}</td></tr>
            <tr><td>Platform</td><td>{systemInfo.platform}</td></tr>
            <tr><td>Uptime</td><td>{systemInfo.uptime}</td></tr>
          </tbody>
        </table>
      ) : <p>Loading...</p>}
    </div>
  )
}
