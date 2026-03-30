import { useState, useEffect, useCallback } from 'react'
import { apiFetch } from '../api/client'
import { pushToast } from '../components/Toast'

export interface RecordingRule {
  id: string
  camera_id: string
  name: string
  mode: 'always' | 'events'
  days: string
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

function toastError(title: string, err?: unknown) {
  pushToast({
    id: `${title}-${Date.now()}`,
    type: 'error',
    title,
    message: err instanceof Error ? err.message : 'An unexpected error occurred',
    timestamp: new Date(),
  })
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
      else toastError('Failed to load recording rules')
      if (statusRes.ok) setStatus(await statusRes.json())
      else toastError('Failed to load recording status')
    } catch (err) {
      toastError('Failed to load recording rules', err)
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
    try {
      const res = await apiFetch(`/cameras/${cameraId}/recording-rules`, {
        method: 'POST',
        body: JSON.stringify(payload),
      })
      if (res.ok) await refresh()
      else toastError('Failed to create recording rule')
      return res
    } catch (err) {
      toastError('Failed to create recording rule', err)
    }
  }

  const updateRule = async (ruleId: string, payload: CreateRulePayload) => {
    try {
      const res = await apiFetch(`/recording-rules/${ruleId}`, {
        method: 'PUT',
        body: JSON.stringify(payload),
      })
      if (res.ok) await refresh()
      else toastError('Failed to update recording rule')
      return res
    } catch (err) {
      toastError('Failed to update recording rule', err)
    }
  }

  const deleteRule = async (ruleId: string) => {
    try {
      const res = await apiFetch(`/recording-rules/${ruleId}`, {
        method: 'DELETE',
      })
      if (res.ok) await refresh()
      else toastError('Failed to delete recording rule')
      return res
    } catch (err) {
      toastError('Failed to delete recording rule', err)
    }
  }

  return { rules, status, loading, refresh, createRule, updateRule, deleteRule }
}
