import { useState, useEffect, useCallback } from 'react'
import { apiFetch } from '../api/client'

export interface RecordingRule {
  id: string
  camera_id: string
  name: string
  mode: 'always' | 'events'
  days: string // JSON array string from API, e.g. "[0,1,2,3,4,5,6]"
  start_time: string
  end_time: string
  post_event_seconds: number
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface RecordingStatus {
  effective_mode: string
  motion_state: string
  active_rules: string[]
  recording: boolean
}

export interface CreateRulePayload {
  name: string
  mode: 'always' | 'events'
  days: number[]
  start_time: string
  end_time: string
  post_event_seconds: number
  enabled: boolean
}

export function useRecordingRules(cameraId: string | null) {
  const [rules, setRules] = useState<RecordingRule[]>([])
  const [status, setStatus] = useState<RecordingStatus | null>(null)
  const [loading, setLoading] = useState(false)

  const refresh = useCallback(async () => {
    if (!cameraId) return
    setLoading(true)
    try {
      const [rulesRes, statusRes] = await Promise.all([
        apiFetch(`/cameras/${cameraId}/recording-rules`),
        apiFetch(`/cameras/${cameraId}/recording-status`),
      ])
      if (rulesRes.ok) setRules(await rulesRes.json())
      if (statusRes.ok) setStatus(await statusRes.json())
    } finally {
      setLoading(false)
    }
  }, [cameraId])

  useEffect(() => {
    if (cameraId) refresh()
    else {
      setRules([])
      setStatus(null)
    }
  }, [cameraId, refresh])

  const createRule = async (payload: CreateRulePayload) => {
    if (!cameraId) return
    const res = await apiFetch(`/cameras/${cameraId}/recording-rules`, {
      method: 'POST',
      body: JSON.stringify(payload),
    })
    if (res.ok) await refresh()
    return res
  }

  const updateRule = async (ruleId: string, payload: CreateRulePayload) => {
    const res = await apiFetch(`/recording-rules/${ruleId}`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    })
    if (res.ok) await refresh()
    return res
  }

  const deleteRule = async (ruleId: string) => {
    const res = await apiFetch(`/recording-rules/${ruleId}`, {
      method: 'DELETE',
    })
    if (res.ok) await refresh()
    return res
  }

  return { rules, status, loading, refresh, createRule, updateRule, deleteRule }
}
