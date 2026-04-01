# Recording & Stream Management Bug Fixes

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 11 confirmed bugs across the recording rules UI, recordings page, playback page, and server-side stream monitoring.

**Architecture:** All client-side fixes target existing React components and hooks. Error handling is unified through the existing `pushToast()` system. Fetch lifecycle bugs are fixed with `AbortController` cleanup in `useEffect` hooks. Server-side fix extends the camera status monitor to track per-stream health.

**Tech Stack:** React/TypeScript (client), Go (server), existing Toast notification system, existing test patterns (Go `testing` package)

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `ui/src/components/RecordingRules.tsx` | Modify | Add post_event_seconds UI control |
| `ui/src/hooks/useRecordingRules.ts` | Modify | Add error handling to refresh/CRUD |
| `ui/src/pages/Recordings.tsx` | Modify | Fix silent errors, AbortController, timezone, clip gap detection |
| `ui/src/pages/Playback.tsx` | Modify | Fix seekRelative side effects, AbortController, timeline marker |
| `internal/nvr/nvr.go` | Modify | Extend camera status monitor for sub-streams |
| `internal/nvr/api/streams.go` | Modify | Add RTSP URL validation |
| `internal/nvr/api/streams_test.go` | Create | Tests for stream URL validation |

---

### Task 1: Fix post_event_seconds hardcoded to 30 in RecordingRules form

**Files:**
- Modify: `ui/src/components/RecordingRules.tsx:87-102`

The `RuleForm` component hardcodes `post_event_seconds: 30` in its `handleSubmit`. Users cannot configure how long recording continues after motion stops. The backend already supports 0-3600.

- [ ] **Step 1: Add post_event_seconds state and form field**

In `RecordingRules.tsx`, inside the `RuleForm` component (line 87), add state for `postEventSeconds` and include it in the submit payload. Also add a slider/input visible only when `mode === 'events'`.

Replace lines 87-102:
```typescript
function RuleForm({ initial, onSave, onCancel }: RuleFormProps) {
  const [name, setName] = useState(initial?.name ?? '')
  const [mode, setMode] = useState<'always' | 'events'>(initial?.mode ?? 'always')
  const [days, setDays] = useState<number[]>(initial ? parseDays(initial.days) : [0, 1, 2, 3, 4, 5, 6])
  const [startTime, setStartTime] = useState(initial?.start_time ?? '00:00')
  const [endTime, setEndTime] = useState(initial?.end_time ?? '23:59')
  const [enabled, setEnabled] = useState(initial?.enabled ?? true)
  const [postEventSeconds, setPostEventSeconds] = useState(initial?.post_event_seconds ?? 30)

  const toggleDay = (d: number) => {
    setDays(prev => prev.includes(d) ? prev.filter(x => x !== d) : [...prev, d].sort())
  }

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    onSave({ name, mode, days, start_time: startTime, end_time: endTime, post_event_seconds: postEventSeconds, enabled })
  }
```

Then replace the events-mode note block (lines 222-227) with an input control:

```typescript
      {/* Post-event seconds (events mode only) */}
      {mode === 'events' && (
        <div className="mb-4">
          <label className="block text-xs font-medium text-nvr-text-secondary mb-1">
            Post-event recording: {postEventSeconds}s
          </label>
          <input
            type="range"
            min={0}
            max={300}
            step={5}
            value={postEventSeconds}
            onChange={e => setPostEventSeconds(Number(e.target.value))}
            className="w-full accent-nvr-accent"
          />
          <div className="flex justify-between text-[10px] text-nvr-text-muted mt-0.5">
            <span>0s (stop immediately)</span>
            <span>300s (5 min)</span>
          </div>
          <p className="text-xs text-nvr-text-muted mt-1">How long to keep recording after motion stops.</p>
        </div>
      )}
```

- [ ] **Step 2: Verify in browser**

Run: `cd ui && npm run dev`

Open the recording rules for any camera. Switch mode to "Events Only" and verify:
- The slider appears with default value 30
- Editing an existing rule shows the saved value
- The slider range is 0-300 with step 5
- Submitting sends the correct value (check Network tab)

- [ ] **Step 3: Commit**

```bash
git add ui/src/components/RecordingRules.tsx
git commit -m "fix: expose post_event_seconds control in recording rule form"
```

---

### Task 2: Replace silent error swallowing with toast notifications in Recordings.tsx

**Files:**
- Modify: `ui/src/pages/Recordings.tsx`

There are 8 locations where errors are silently caught with `.catch(() => [])` or `catch { /* silently fail */ }`. Users get no feedback when API calls fail.

- [ ] **Step 1: Add pushToast import**

At the top of `Recordings.tsx`, add the import:

```typescript
import { pushToast } from '../components/Toast'
```

- [ ] **Step 2: Add a toast helper**

After the imports section (around line 11), add:

```typescript
function toastError(title: string, err?: unknown) {
  pushToast({
    id: `${title}-${Date.now()}`,
    type: 'error',
    title,
    message: err instanceof Error ? err.message : 'An unexpected error occurred',
    timestamp: new Date(),
  })
}
```

- [ ] **Step 3: Fix fetchCameraRecordings (line 236)**

Replace:
```typescript
      .catch(() => [])
```

With:
```typescript
      .catch(err => {
        toastError('Failed to load recordings', err)
        return []
      })
```

- [ ] **Step 4: Fix motion events fetch (line 274)**

Replace:
```typescript
      .catch(() => setMotionEvents([]))
```

With:
```typescript
      .catch(err => {
        toastError('Failed to load motion events', err)
        setMotionEvents([])
      })
```

- [ ] **Step 5: Fix recording dates fetch (line 298)**

Replace:
```typescript
      .catch(() => setRecordingDates(new Set()))
```

With:
```typescript
      .catch(err => {
        toastError('Failed to load recording dates', err)
        setRecordingDates(new Set())
      })
```

- [ ] **Step 6: Fix motion dates fetch (line 318)**

Replace:
```typescript
      .catch(() => setMotionDates(new Set()))
```

With:
```typescript
      .catch(err => {
        toastError('Failed to load motion dates', err)
        setMotionDates(new Set())
      })
```

- [ ] **Step 7: Fix saved clips fetch (line 363)**

Replace:
```typescript
      .catch(() => setSavedClips([]))
```

With:
```typescript
      .catch(err => {
        toastError('Failed to load saved clips', err)
        setSavedClips([])
      })
```

- [ ] **Step 8: Fix handleSaveClip (lines 393-394)**

Replace:
```typescript
    } catch {
      // silently fail
    } finally {
```

With:
```typescript
    } catch (err) {
      toastError('Failed to save clip', err)
    } finally {
```

- [ ] **Step 9: Fix handleDeleteSavedClip (lines 407-408)**

Replace:
```typescript
    } catch {
      // silently fail
    }
```

With:
```typescript
    } catch (err) {
      toastError('Failed to delete clip', err)
    }
```

- [ ] **Step 10: Verify in browser**

Open recordings page, disconnect network in DevTools, interact with the page. Verify toast notifications appear for each failure type instead of silent failures.

- [ ] **Step 11: Commit**

```bash
git add ui/src/pages/Recordings.tsx
git commit -m "fix: replace silent error swallowing with toast notifications in Recordings"
```

---

### Task 3: Replace silent error swallowing with toast notifications in Playback.tsx

**Files:**
- Modify: `ui/src/pages/Playback.tsx`

Same pattern as Task 2 but for Playback.tsx. The `.catch(() => {})` on `video.play()` calls are intentional (browsers reject play() promises when interrupted) and should NOT be changed. Only API fetch errors need fixing.

- [ ] **Step 1: Add pushToast import and helper**

At the top of `Playback.tsx`, add:

```typescript
import { pushToast } from '../components/Toast'
```

After the helpers section (around line 27), add:

```typescript
function toastError(title: string, err?: unknown) {
  pushToast({
    id: `${title}-${Date.now()}`,
    type: 'error',
    title,
    message: err instanceof Error ? err.message : 'An unexpected error occurred',
    timestamp: new Date(),
  })
}
```

- [ ] **Step 2: Fix camera ranges fetch (line 549)**

Replace:
```typescript
        .catch(() => {})
```

With:
```typescript
        .catch(err => toastError('Failed to load recordings for camera', err))
```

- [ ] **Step 3: Fix motion events fetch (line 569)**

Replace:
```typescript
        .catch(() => [])
```

With:
```typescript
        .catch(err => {
          toastError('Failed to load motion events', err)
          return []
        })
```

- [ ] **Step 4: Verify in browser**

Open Playback page, disable network, add cameras. Verify toast errors appear for recording/motion fetch failures. Verify video play/pause still works smoothly (the `.catch(() => {})` on `video.play()` should be left alone).

- [ ] **Step 5: Commit**

```bash
git add ui/src/pages/Playback.tsx
git commit -m "fix: replace silent error swallowing with toast notifications in Playback"
```

---

### Task 4: Add AbortController cleanup to Recordings.tsx fetch effects

**Files:**
- Modify: `ui/src/pages/Recordings.tsx`

Multiple `useEffect` hooks fire fetch requests without `AbortController`. When the component unmounts or dependencies change, stale responses update unmounted state causing memory leaks.

- [ ] **Step 1: Fix single-camera recordings fetch (lines 240-257)**

Replace the useEffect:
```typescript
  useEffect(() => {
    if (isAllCameras || !mediamtxPath || !date) {
      if (!isAllCameras) {
        setTimelineRanges([])
        setHasRecordings(false)
      }
      return
    }

    setLoadingRecordings(true)

    fetchCameraRecordings(mediamtxPath, date)
      .then(ranges => {
        setTimelineRanges(ranges)
        setHasRecordings(ranges.length > 0)
      })
      .finally(() => setLoadingRecordings(false))
  }, [mediamtxPath, date, isAllCameras, fetchCameraRecordings])
```

With:
```typescript
  useEffect(() => {
    if (isAllCameras || !mediamtxPath || !date) {
      if (!isAllCameras) {
        setTimelineRanges([])
        setHasRecordings(false)
      }
      return
    }

    let cancelled = false
    setLoadingRecordings(true)

    fetchCameraRecordings(mediamtxPath, date)
      .then(ranges => {
        if (cancelled) return
        setTimelineRanges(ranges)
        setHasRecordings(ranges.length > 0)
      })
      .finally(() => {
        if (!cancelled) setLoadingRecordings(false)
      })

    return () => { cancelled = true }
  }, [mediamtxPath, date, isAllCameras, fetchCameraRecordings])
```

- [ ] **Step 2: Fix motion events fetch (lines 260-275)**

Replace:
```typescript
  useEffect(() => {
    if (isAllCameras || !selectedCamera || !date) {
      setMotionEvents([])
      return
    }

    let url = `/cameras/${selectedCamera}/motion-events?date=${date}`
    if (objectClassFilter) {
      url += `&object_class=${encodeURIComponent(objectClassFilter)}`
    }

    apiFetch(url)
      .then(res => res.ok ? res.json() : [])
      .then((data: MotionEvent[]) => setMotionEvents(data))
      .catch(err => {
        toastError('Failed to load motion events', err)
        setMotionEvents([])
      })
  }, [selectedCamera, date, isAllCameras, objectClassFilter])
```

With:
```typescript
  useEffect(() => {
    if (isAllCameras || !selectedCamera || !date) {
      setMotionEvents([])
      return
    }

    let cancelled = false
    let url = `/cameras/${selectedCamera}/motion-events?date=${date}`
    if (objectClassFilter) {
      url += `&object_class=${encodeURIComponent(objectClassFilter)}`
    }

    apiFetch(url)
      .then(res => res.ok ? res.json() : [])
      .then((data: MotionEvent[]) => {
        if (!cancelled) setMotionEvents(data)
      })
      .catch(err => {
        if (!cancelled) {
          toastError('Failed to load motion events', err)
          setMotionEvents([])
        }
      })

    return () => { cancelled = true }
  }, [selectedCamera, date, isAllCameras, objectClassFilter])
```

- [ ] **Step 3: Fix recording dates fetch (lines 278-299)**

Replace:
```typescript
  useEffect(() => {
    if (isAllCameras || !mediamtxPath) {
      setRecordingDates(new Set())
      return
    }

    fetch(`http://${window.location.hostname}:9997/v3/recordings/get/${mediamtxPath}`)
      .then(res => res.ok ? res.json() : null)
      .then((data: RecordingList | null) => {
        if (!data || !data.segments) {
          setRecordingDates(new Set())
          return
        }
        const dates = new Set<string>()
        data.segments.forEach(s => {
          const d = new Date(s.start)
          dates.add(d.toISOString().split('T')[0])
        })
        setRecordingDates(dates)
      })
      .catch(err => {
        toastError('Failed to load recording dates', err)
        setRecordingDates(new Set())
      })
  }, [mediamtxPath, isAllCameras])
```

With:
```typescript
  useEffect(() => {
    if (isAllCameras || !mediamtxPath) {
      setRecordingDates(new Set())
      return
    }

    let cancelled = false

    fetch(`http://${window.location.hostname}:9997/v3/recordings/get/${mediamtxPath}`)
      .then(res => res.ok ? res.json() : null)
      .then((data: RecordingList | null) => {
        if (cancelled) return
        if (!data || !data.segments) {
          setRecordingDates(new Set())
          return
        }
        const dates = new Set<string>()
        data.segments.forEach(s => {
          const d = new Date(s.start)
          dates.add(d.toISOString().split('T')[0])
        })
        setRecordingDates(dates)
      })
      .catch(err => {
        if (!cancelled) {
          toastError('Failed to load recording dates', err)
          setRecordingDates(new Set())
        }
      })

    return () => { cancelled = true }
  }, [mediamtxPath, isAllCameras])
```

- [ ] **Step 4: Fix motion dates fetch (lines 302-319)**

Replace:
```typescript
  useEffect(() => {
    if (isAllCameras || !selectedCamera) {
      setMotionDates(new Set())
      return
    }

    apiFetch(`/cameras/${selectedCamera}/motion-events?days=90`)
      .then(res => res.ok ? res.json() : [])
      .then((events: MotionEvent[]) => {
        const dates = new Set<string>()
        events.forEach(ev => {
          const d = new Date(ev.started_at)
          dates.add(d.toISOString().split('T')[0])
        })
        setMotionDates(dates)
      })
      .catch(err => {
        toastError('Failed to load motion dates', err)
        setMotionDates(new Set())
      })
  }, [selectedCamera, isAllCameras])
```

With:
```typescript
  useEffect(() => {
    if (isAllCameras || !selectedCamera) {
      setMotionDates(new Set())
      return
    }

    let cancelled = false

    apiFetch(`/cameras/${selectedCamera}/motion-events?days=90`)
      .then(res => res.ok ? res.json() : [])
      .then((events: MotionEvent[]) => {
        if (cancelled) return
        const dates = new Set<string>()
        events.forEach(ev => {
          const d = new Date(ev.started_at)
          dates.add(d.toISOString().split('T')[0])
        })
        setMotionDates(dates)
      })
      .catch(err => {
        if (!cancelled) {
          toastError('Failed to load motion dates', err)
          setMotionDates(new Set())
        }
      })

    return () => { cancelled = true }
  }, [selectedCamera, isAllCameras])
```

- [ ] **Step 5: Fix all-cameras parallel fetch (lines 322-352)**

Replace:
```typescript
  useEffect(() => {
    if (!isAllCameras || !date || cameras.length === 0) {
      if (isAllCameras) setAllCameraRanges([])
      return
    }

    const initial: AllCameraRanges[] = cameras
      .filter(c => c.mediamtx_path)
      .map(c => ({
        cameraId: c.id,
        cameraName: c.name,
        mediamtxPath: c.mediamtx_path,
        ranges: [],
        loading: true,
      }))
    setAllCameraRanges(initial)

    initial.forEach(cam => {
      fetchCameraRecordings(cam.mediamtxPath, date).then(ranges => {
        setAllCameraRanges(prev =>
          prev.map(c =>
            c.cameraId === cam.cameraId
              ? { ...c, ranges, loading: false }
              : c
          )
        )
      })
    })
  }, [isAllCameras, date, cameras, fetchCameraRecordings])
```

With:
```typescript
  useEffect(() => {
    if (!isAllCameras || !date || cameras.length === 0) {
      if (isAllCameras) setAllCameraRanges([])
      return
    }

    let cancelled = false

    const initial: AllCameraRanges[] = cameras
      .filter(c => c.mediamtx_path)
      .map(c => ({
        cameraId: c.id,
        cameraName: c.name,
        mediamtxPath: c.mediamtx_path,
        ranges: [],
        loading: true,
      }))
    setAllCameraRanges(initial)

    initial.forEach(cam => {
      fetchCameraRecordings(cam.mediamtxPath, date).then(ranges => {
        if (cancelled) return
        setAllCameraRanges(prev =>
          prev.map(c =>
            c.cameraId === cam.cameraId
              ? { ...c, ranges, loading: false }
              : c
          )
        )
      })
    })

    return () => { cancelled = true }
  }, [isAllCameras, date, cameras, fetchCameraRecordings])
```

- [ ] **Step 6: Verify**

Run: `cd ui && npx tsc --noEmit`
Expected: No type errors.

- [ ] **Step 7: Commit**

```bash
git add ui/src/pages/Recordings.tsx
git commit -m "fix: add cancellation cleanup to all fetch effects in Recordings"
```

---

### Task 5: Add AbortController cleanup to Playback.tsx fetch effects

**Files:**
- Modify: `ui/src/pages/Playback.tsx`

- [ ] **Step 1: Fix camera ranges fetch effect (around lines 520-551)**

The useEffect that fetches recording ranges for each selected camera needs cancellation. Find the effect that calls `fetch(...)` for each camera in `selectedCameras` and wraps `setCameraRanges`.

Add `let cancelled = false` at the top of the effect body and guard all `setCameraRanges` calls. Add `return () => { cancelled = true }` as cleanup.

- [ ] **Step 2: Fix motion events fetch effect (around lines 560-577)**

Find the effect that fetches motion events via `Promise.all`. Add cancellation:

Replace:
```typescript
    Promise.all(promises).then((results: MotionEvent[][]) => {
      const all = results.flat()
      all.sort((a, b) => new Date(a.started_at).getTime() - new Date(b.started_at).getTime())
      setMotionEvents(all)
    })
```

With:
```typescript
    let cancelled = false

    Promise.all(promises).then((results: MotionEvent[][]) => {
      if (cancelled) return
      const all = results.flat()
      all.sort((a, b) => new Date(a.started_at).getTime() - new Date(b.started_at).getTime())
      setMotionEvents(all)
    })

    return () => { cancelled = true }
```

- [ ] **Step 3: Verify**

Run: `cd ui && npx tsc --noEmit`
Expected: No type errors.

- [ ] **Step 4: Commit**

```bash
git add ui/src/pages/Playback.tsx
git commit -m "fix: add cancellation cleanup to fetch effects in Playback"
```

---

### Task 6: Fix seekRelative side effects inside setState updater

**Files:**
- Modify: `ui/src/pages/Playback.tsx:681-692`

The `seekRelative` function performs side effects (pausing videos, resetting refs) inside a `setPlaybackTime` updater function. React may call updater functions multiple times or defer them. Side effects should happen outside the updater.

- [ ] **Step 1: Move side effects out of setState updater**

Replace lines 681-692:
```typescript
  const seekRelative = useCallback((seconds: number) => {
    setPlaybackTime(prev => {
      if (!prev) return prev
      const newTime = new Date(prev.getTime() + seconds * 1000)
      // Pause all videos and trigger re-sync
      videoRefs.current.forEach(video => video.pause())
      readyCamerasRef.current = new Set()
      syncPendingRef.current = true
      videoStartTimeRef.current = newTime
      return newTime
    })
  }, [])
```

With:
```typescript
  const seekRelative = useCallback((seconds: number) => {
    setPlaybackTime(prev => {
      if (!prev) return prev
      return new Date(prev.getTime() + seconds * 1000)
    })
  }, [])
```

This removes the side effects from the updater. The video reload is already handled by the `CameraTile` component's `useEffect` on `src` change (lines 70-80 of Playback.tsx) — when `playbackTime` changes, `src` is recomputed via `useMemo`, which triggers the video reload effect. The `lastSeekRef` check ensures it only reloads when the src actually changes.

- [ ] **Step 2: Verify keyboard seek still works**

Open Playback page, select cameras, start playback, use ArrowLeft/ArrowRight. Verify:
- Videos pause and reload at new time
- Sync mechanism works (all cameras resume together)
- No console errors

- [ ] **Step 3: Commit**

```bash
git add ui/src/pages/Playback.tsx
git commit -m "fix: move side effects out of seekRelative setState updater"
```

---

### Task 7: Fix motion event timezone issue in calendar dates

**Files:**
- Modify: `ui/src/pages/Recordings.tsx:310-315`

Motion event dates are extracted using `toISOString().split('T')[0]` which converts to UTC. An event at `2026-03-30T23:30:00-05:00` displays on `2026-03-31` in UTC, but the user expects `2026-03-30`.

The recording dates fetch at line 293-294 has the same issue.

- [ ] **Step 1: Fix motion dates extraction (lines 310-315)**

Replace:
```typescript
      .then((events: MotionEvent[]) => {
        const dates = new Set<string>()
        events.forEach(ev => {
          const d = new Date(ev.started_at)
          dates.add(d.toISOString().split('T')[0])
        })
        setMotionDates(dates)
      })
```

With:
```typescript
      .then((events: MotionEvent[]) => {
        const dates = new Set<string>()
        events.forEach(ev => {
          const d = new Date(ev.started_at)
          const y = d.getFullYear()
          const m = String(d.getMonth() + 1).padStart(2, '0')
          const day = String(d.getDate()).padStart(2, '0')
          dates.add(`${y}-${m}-${day}`)
        })
        setMotionDates(dates)
      })
```

- [ ] **Step 2: Fix recording dates extraction (lines 291-295)**

Replace:
```typescript
        const dates = new Set<string>()
        data.segments.forEach(s => {
          const d = new Date(s.start)
          dates.add(d.toISOString().split('T')[0])
        })
```

With:
```typescript
        const dates = new Set<string>()
        data.segments.forEach(s => {
          const d = new Date(s.start)
          const y = d.getFullYear()
          const m = String(d.getMonth() + 1).padStart(2, '0')
          const day = String(d.getDate()).padStart(2, '0')
          dates.add(`${y}-${m}-${day}`)
        })
```

- [ ] **Step 3: Verify**

Run: `cd ui && npx tsc --noEmit`
Expected: No type errors.

Check in browser: calendar dots should align with the user's local timezone, not UTC.

- [ ] **Step 4: Commit**

```bash
git add ui/src/pages/Recordings.tsx
git commit -m "fix: use local timezone for calendar date extraction instead of UTC"
```

---

### Task 8: Fix clip gap detection false positives

**Files:**
- Modify: `ui/src/pages/Recordings.tsx:530-578`

The `clipValidation` memo checks for gaps but doesn't account for `fetchCameraRecordings` already merging ranges within 30s (line 227). The gap detection uses the same 30s threshold but checks against `clipStartMs`/`clipEndMs` boundaries, which creates false positives when the clip boundary falls within a recording range.

The real issue: the start/end gap checks compare clip boundaries against recording boundaries, but the clip can start mid-recording. A gap at the start should only be flagged if the clip starts BEFORE the first recording range, not if the clip starts within a range.

- [ ] **Step 1: Fix the clip validation logic**

Replace lines 530-578:
```typescript
  const clipValidation = useMemo(() => {
    if (!clipStart || !clipEnd || timelineRanges.length === 0) return null

    const clipStartMs = clipStart.getTime()
    const clipEndMs = clipEnd.getTime()

    // Check if any recording range overlaps with the clip range
    const overlappingRanges = timelineRanges.filter(r => {
      const rStart = new Date(r.start).getTime()
      const rEnd = new Date(r.end).getTime()
      return rStart < clipEndMs && rEnd > clipStartMs
    })

    if (overlappingRanges.length === 0) {
      return { type: 'error' as const, message: 'No recordings in selected range' }
    }

    // Check if there are gaps within the clip range
    // Sort overlapping ranges and check for gaps between them
    const sorted = [...overlappingRanges].sort((a, b) =>
      new Date(a.start).getTime() - new Date(b.start).getTime()
    )

    let hasGaps = false
    // Check gap at the start
    if (new Date(sorted[0].start).getTime() > clipStartMs + 30000) {
      hasGaps = true
    }
    // Check gaps between ranges
    for (let i = 1; i < sorted.length; i++) {
      const prevEnd = new Date(sorted[i - 1].end).getTime()
      const curStart = new Date(sorted[i].start).getTime()
      if (curStart - prevEnd > 30000) {
        hasGaps = true
        break
      }
    }
    // Check gap at the end
    if (new Date(sorted[sorted.length - 1].end).getTime() < clipEndMs - 30000) {
      hasGaps = true
    }

    if (hasGaps) {
      return { type: 'warning' as const, message: 'Clip includes gaps in recording. Footage will skip missing portions.' }
    }

    return { type: 'ok' as const, message: '' }
  }, [clipStart, clipEnd, timelineRanges])
```

With:
```typescript
  const clipValidation = useMemo(() => {
    if (!clipStart || !clipEnd || timelineRanges.length === 0) return null

    const clipStartMs = clipStart.getTime()
    const clipEndMs = clipEnd.getTime()
    const GAP_THRESHOLD_MS = 30000

    // Find recording ranges that overlap the clip window
    const overlapping = timelineRanges
      .filter(r => {
        const rStart = new Date(r.start).getTime()
        const rEnd = new Date(r.end).getTime()
        return rStart < clipEndMs && rEnd > clipStartMs
      })
      .sort((a, b) => new Date(a.start).getTime() - new Date(b.start).getTime())

    if (overlapping.length === 0) {
      return { type: 'error' as const, message: 'No recordings in selected range' }
    }

    // Merge overlapping/adjacent ranges (within threshold) clipped to clip window
    const merged: { start: number; end: number }[] = []
    for (const r of overlapping) {
      const rStart = Math.max(new Date(r.start).getTime(), clipStartMs)
      const rEnd = Math.min(new Date(r.end).getTime(), clipEndMs)
      if (merged.length > 0 && rStart - merged[merged.length - 1].end <= GAP_THRESHOLD_MS) {
        merged[merged.length - 1].end = Math.max(merged[merged.length - 1].end, rEnd)
      } else {
        merged.push({ start: rStart, end: rEnd })
      }
    }

    // Check if merged ranges cover the full clip window
    const hasStartGap = merged[0].start - clipStartMs > GAP_THRESHOLD_MS
    const hasEndGap = clipEndMs - merged[merged.length - 1].end > GAP_THRESHOLD_MS
    const hasMiddleGaps = merged.length > 1

    if (hasStartGap || hasEndGap || hasMiddleGaps) {
      return { type: 'warning' as const, message: 'Clip includes gaps in recording. Footage will skip missing portions.' }
    }

    return { type: 'ok' as const, message: '' }
  }, [clipStart, clipEnd, timelineRanges])
```

- [ ] **Step 2: Verify**

Run: `cd ui && npx tsc --noEmit`
Expected: No type errors.

Test in browser: create a clip that spans a single continuous recording range. Verify no false gap warning appears. Then create a clip that spans a known gap. Verify the warning appears.

- [ ] **Step 3: Commit**

```bash
git add ui/src/pages/Recordings.tsx
git commit -m "fix: merge overlapping ranges in clip gap detection to prevent false positives"
```

---

### Task 9: Fix timeline marker DOM update in Playback.tsx

**Files:**
- Modify: `ui/src/pages/Playback.tsx:585-613`

The `setInterval` in the timeline marker effect captures `date` in its closure but `date` is not in the dependency array. If the user changes the date while playing, the marker calculates position using the stale date.

- [ ] **Step 1: Add date to the dependency array and cleanup guard**

Replace lines 585-613:
```typescript
  useEffect(() => {
    if (playing && videoStartTimeRef.current) {
      const firstVideo = videoRefs.current.values().next().value
      if (firstVideo) {
        timeUpdateIntervalRef.current = setInterval(() => {
          if (videoStartTimeRef.current && firstVideo && !firstVideo.paused) {
            const wallTime = new Date(videoStartTimeRef.current.getTime() + firstVideo.currentTime * 1000)
            livePlaybackTimeRef.current = wallTime
            // Update timeline marker position directly in DOM (no React re-render)
            if (timelineMarkerRef.current) {
              const dayStart = new Date(date + 'T00:00:00')
              const dayMs = 24 * 60 * 60 * 1000
              const TOTAL_HEIGHT = Math.min(960, typeof window !== 'undefined' ? window.innerHeight - 200 : 960)
              const px = ((wallTime.getTime() - dayStart.getTime()) / dayMs) * TOTAL_HEIGHT
              timelineMarkerRef.current.style.top = `${Math.max(0, Math.min(TOTAL_HEIGHT, px))}px`
              timelineMarkerRef.current.style.display = px >= 0 && px <= TOTAL_HEIGHT ? 'block' : 'none'
            }
          }
        }, 250)
      }
    }

    return () => {
      if (timeUpdateIntervalRef.current) {
        clearInterval(timeUpdateIntervalRef.current)
        timeUpdateIntervalRef.current = null
      }
    }
  }, [playing])
```

With:
```typescript
  useEffect(() => {
    if (playing && videoStartTimeRef.current) {
      const firstVideo = videoRefs.current.values().next().value
      if (firstVideo) {
        const currentDate = date
        timeUpdateIntervalRef.current = setInterval(() => {
          if (videoStartTimeRef.current && firstVideo && !firstVideo.paused) {
            const wallTime = new Date(videoStartTimeRef.current.getTime() + firstVideo.currentTime * 1000)
            livePlaybackTimeRef.current = wallTime
            if (timelineMarkerRef.current) {
              const dayStart = new Date(currentDate + 'T00:00:00')
              const dayMs = 24 * 60 * 60 * 1000
              const TOTAL_HEIGHT = Math.min(960, typeof window !== 'undefined' ? window.innerHeight - 200 : 960)
              const px = ((wallTime.getTime() - dayStart.getTime()) / dayMs) * TOTAL_HEIGHT
              timelineMarkerRef.current.style.top = `${Math.max(0, Math.min(TOTAL_HEIGHT, px))}px`
              timelineMarkerRef.current.style.display = px >= 0 && px <= TOTAL_HEIGHT ? 'block' : 'none'
            }
          }
        }, 250)
      }
    }

    return () => {
      if (timeUpdateIntervalRef.current) {
        clearInterval(timeUpdateIntervalRef.current)
        timeUpdateIntervalRef.current = null
      }
    }
  }, [playing, date])
```

- [ ] **Step 2: Verify**

Run: `cd ui && npx tsc --noEmit`
Expected: No type errors.

- [ ] **Step 3: Commit**

```bash
git add ui/src/pages/Playback.tsx
git commit -m "fix: include date in timeline marker effect deps to prevent stale closure"
```

---

### Task 10: Add RTSP URL format validation to stream creation API

**Files:**
- Modify: `internal/nvr/api/streams.go:44-79`
- Create: `internal/nvr/api/streams_test.go`

Stream creation accepts any string as `rtsp_url` without validating it's a valid RTSP URL. Bad URLs are stored silently and only discovered at runtime.

- [ ] **Step 1: Write the failing test**

Create `internal/nvr/api/streams_test.go`:

```go
package api

import (
	"testing"
)

func TestValidateStreamRTSPURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid rtsp", "rtsp://192.168.1.100:554/stream1", false},
		{"valid rtsps", "rtsps://192.168.1.100:554/stream1", false},
		{"valid with credentials", "rtsp://admin:pass@192.168.1.100:554/cam/realmonitor", false},
		{"empty url", "", true},
		{"http url", "http://example.com/stream", true},
		{"no scheme", "192.168.1.100:554/stream", true},
		{"ftp url", "ftp://example.com/file", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStreamURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateStreamURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestValidateStreamRTSPURL -v`
Expected: FAIL — `validateStreamURL` is not defined.

- [ ] **Step 3: Implement validation**

In `internal/nvr/api/streams.go`, add the validation function and call it in Create:

Add after the imports:
```go
import (
	"fmt"
	"net/url"
	"strings"
)
```

Add the validation function (before the `Create` method):
```go
func validateStreamURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("rtsp_url is required")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "rtsp" && scheme != "rtsps" {
		return fmt.Errorf("URL scheme must be rtsp:// or rtsps://, got %q", u.Scheme)
	}
	return nil
}
```

In the `Create` method, add validation after `ShouldBindJSON` (around line 55):
```go
	if err := validateStreamURL(req.RTSPURL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestValidateStreamRTSPURL -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/api/streams.go internal/nvr/api/streams_test.go
git commit -m "fix: validate RTSP URL scheme on stream creation"
```

---

### Task 11: Extend camera status monitor to track sub-stream health

**Files:**
- Modify: `internal/nvr/nvr.go` (the `runCameraStatusMonitor` function)

Currently, the status monitor only checks the camera's main MediaMTX path. If a sub-stream (used for AI or recording) goes offline, no event is published.

- [ ] **Step 1: Read the current runCameraStatusMonitor implementation**

Read `internal/nvr/nvr.go` starting around line 204 to understand the full function. Identify where it fetches paths from MediaMTX and how it builds the status map.

- [ ] **Step 2: Modify the monitor to include sub-stream paths**

The current implementation fetches all paths from `/v3/paths/list` and checks which ones match camera MediaMTX paths. Sub-stream paths follow the pattern `<mediamtx_path>~<prefix>`. The path list from MediaMTX already includes these — they just aren't being checked.

In the section where the monitor iterates over cameras to check status (the loop that compares `cam.MediaMTXPath` against the fetched path readiness map), extend it to also check any paths that start with `cam.MediaMTXPath + "~"`.

After the main path check, add logic to detect sub-stream status:
```go
// Check sub-stream paths (format: <main_path>~<prefix>)
subPrefix := cam.MediaMTXPath + "~"
for pathName, ready := range currentPaths {
    if !strings.HasPrefix(pathName, subPrefix) {
        continue
    }
    prevReady, existed := prevPaths[pathName]
    if !existed {
        continue // first observation, skip event
    }
    if prevReady && !ready {
        nvrLogWarn("sub-stream offline: %s (camera %s)", pathName, cam.Name)
        if n.eventPub != nil {
            n.eventPub.PublishCameraOffline(cam.Name + " (" + strings.TrimPrefix(pathName, cam.MediaMTXPath+"~") + ")")
        }
    } else if !prevReady && ready {
        nvrLogInfo("sub-stream online: %s (camera %s)", pathName, cam.Name)
        if n.eventPub != nil {
            n.eventPub.PublishCameraOnline(cam.Name + " (" + strings.TrimPrefix(pathName, cam.MediaMTXPath+"~") + ")")
        }
    }
}
```

- [ ] **Step 3: Verify compilation**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./...`
Expected: Build succeeds with no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/nvr.go
git commit -m "fix: extend camera status monitor to track sub-stream health"
```

---

### Task 12: Add error handling to useRecordingRules hook

**Files:**
- Modify: `ui/src/hooks/useRecordingRules.ts`

The hook's `refresh()` function doesn't handle errors — if both API calls fail, the `catch` is unhandled. CRUD operations also lack error feedback.

- [ ] **Step 1: Add toast notifications for failures**

Replace the full file content:

```typescript
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
```

- [ ] **Step 2: Verify**

Run: `cd ui && npx tsc --noEmit`
Expected: No type errors.

- [ ] **Step 3: Commit**

```bash
git add ui/src/hooks/useRecordingRules.ts
git commit -m "fix: add error handling with toast notifications to useRecordingRules hook"
```

---

### Task 13: Final verification

- [ ] **Step 1: Run Go tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/... -v -count=1`
Expected: All tests pass.

- [ ] **Step 2: Run TypeScript type check**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/ui && npx tsc --noEmit`
Expected: No type errors.

- [ ] **Step 3: Build the Go server**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./...`
Expected: Build succeeds.

- [ ] **Step 4: Build the UI**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/ui && npm run build`
Expected: Build succeeds.

- [ ] **Step 5: Manual smoke test**

Start the server and UI. Verify:
1. Recording rules form shows post-event seconds slider when mode is "Events"
2. Toast notifications appear when API calls fail (test by disconnecting network)
3. Recordings page handles camera switching without stale data
4. Playback keyboard seek (ArrowLeft/ArrowRight) works correctly
5. Calendar dots reflect local timezone dates
6. Clip validation doesn't show false gap warnings for continuous recordings
