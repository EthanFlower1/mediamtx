import { useState, useCallback } from 'react'

// --- Types matching the Go DSL ---

type Operator = 'AND' | 'OR' | 'NOT'
type ClauseType =
  | 'clip'
  | 'object'
  | 'lpr'
  | 'time'
  | 'camera'
  | 'time_of_day'
  | 'day_of_week'
  | 'confidence'

interface QueryNode {
  op?: Operator
  children?: QueryNode[]
  type?: ClauseType
  clip_text?: string
  object_class?: string
  plate_text?: string
  start?: string
  end?: string
  camera_ids?: string[]
  time_of_day_start?: string
  time_of_day_end?: string
  days_of_week?: number[]
  min_confidence?: number
}

interface ForensicResult {
  id: string
  camera_id: string
  camera_name: string
  timestamp: string
  matched_classes?: string[]
  matched_plate?: string
  score: number
  clip_similarity?: number
  confidence: number
  thumbnail_path?: string
  snippet_start?: string
  snippet_end?: string
}

interface ResultSet {
  query: string
  total_matches: number
  results: ForensicResult[]
  execution_time_ms: number
}

// --- Helpers ---

const DAY_NAMES = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']
const OBJECT_CLASSES = [
  'person',
  'car',
  'truck',
  'bus',
  'motorcycle',
  'bicycle',
  'dog',
  'cat',
  'backpack',
  'suitcase',
]

// --- Clause Editor ---

function ClauseEditor({
  clause,
  onChange,
  onRemove,
}: {
  clause: QueryNode
  onChange: (c: QueryNode) => void
  onRemove: () => void
}) {
  const type = clause.type || 'clip'

  return (
    <div
      style={{
        border: '1px solid #ddd',
        borderRadius: 6,
        padding: 12,
        marginBottom: 8,
        background: '#f9f9fb',
      }}
    >
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          marginBottom: 8,
        }}
      >
        <select
          value={type}
          onChange={e => {
            const t = e.target.value as ClauseType
            onChange({ type: t })
          }}
          style={{ fontWeight: 'bold' }}
        >
          <option value="clip">CLIP Text Search</option>
          <option value="object">Object Class</option>
          <option value="lpr">License Plate</option>
          <option value="time">Time Range</option>
          <option value="camera">Camera</option>
          <option value="time_of_day">Time of Day</option>
          <option value="day_of_week">Day of Week</option>
          <option value="confidence">Confidence</option>
        </select>
        <button onClick={onRemove} style={{ color: '#c00' }}>
          Remove
        </button>
      </div>

      {type === 'clip' && (
        <input
          type="text"
          placeholder='e.g., "red truck at loading dock"'
          value={clause.clip_text || ''}
          onChange={e => onChange({ ...clause, clip_text: e.target.value })}
          style={{ width: '100%', padding: 6 }}
        />
      )}

      {type === 'object' && (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
          {OBJECT_CLASSES.map(cls => (
            <label key={cls} style={{ marginRight: 8, cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={(clause.object_class || '').split(',').includes(cls)}
                onChange={e => {
                  const current = (clause.object_class || '')
                    .split(',')
                    .filter(Boolean)
                  const next = e.target.checked
                    ? [...current, cls]
                    : current.filter(c => c !== cls)
                  onChange({ ...clause, object_class: next.join(',') })
                }}
              />{' '}
              {cls}
            </label>
          ))}
        </div>
      )}

      {type === 'lpr' && (
        <input
          type="text"
          placeholder='e.g., "ABC123" or "ABC*" for wildcard'
          value={clause.plate_text || ''}
          onChange={e => onChange({ ...clause, plate_text: e.target.value })}
          style={{ width: '100%', padding: 6 }}
        />
      )}

      {type === 'time' && (
        <div style={{ display: 'flex', gap: 8 }}>
          <label>
            Start:{' '}
            <input
              type="datetime-local"
              value={(clause.start || '').slice(0, 16)}
              onChange={e =>
                onChange({
                  ...clause,
                  start: new Date(e.target.value).toISOString(),
                })
              }
            />
          </label>
          <label>
            End:{' '}
            <input
              type="datetime-local"
              value={(clause.end || '').slice(0, 16)}
              onChange={e =>
                onChange({
                  ...clause,
                  end: new Date(e.target.value).toISOString(),
                })
              }
            />
          </label>
        </div>
      )}

      {type === 'camera' && (
        <input
          type="text"
          placeholder="Camera IDs (comma-separated)"
          value={(clause.camera_ids || []).join(',')}
          onChange={e =>
            onChange({
              ...clause,
              camera_ids: e.target.value
                .split(',')
                .map(s => s.trim())
                .filter(Boolean),
            })
          }
          style={{ width: '100%', padding: 6 }}
        />
      )}

      {type === 'time_of_day' && (
        <div style={{ display: 'flex', gap: 8 }}>
          <label>
            From:{' '}
            <input
              type="time"
              value={clause.time_of_day_start || ''}
              onChange={e =>
                onChange({ ...clause, time_of_day_start: e.target.value })
              }
            />
          </label>
          <label>
            To:{' '}
            <input
              type="time"
              value={clause.time_of_day_end || ''}
              onChange={e =>
                onChange({ ...clause, time_of_day_end: e.target.value })
              }
            />
          </label>
          <span style={{ color: '#666', fontSize: 12, alignSelf: 'center' }}>
            (overnight wrapping supported)
          </span>
        </div>
      )}

      {type === 'day_of_week' && (
        <div style={{ display: 'flex', gap: 4 }}>
          {DAY_NAMES.map((name, i) => {
            const day = i + 1
            return (
              <label
                key={day}
                style={{
                  padding: '4px 8px',
                  border: '1px solid #ccc',
                  borderRadius: 4,
                  cursor: 'pointer',
                  background: (clause.days_of_week || []).includes(day)
                    ? '#4a90d9'
                    : '#fff',
                  color: (clause.days_of_week || []).includes(day)
                    ? '#fff'
                    : '#333',
                }}
              >
                <input
                  type="checkbox"
                  hidden
                  checked={(clause.days_of_week || []).includes(day)}
                  onChange={e => {
                    const current = clause.days_of_week || []
                    const next = e.target.checked
                      ? [...current, day]
                      : current.filter(d => d !== day)
                    onChange({ ...clause, days_of_week: next })
                  }}
                />
                {name}
              </label>
            )
          })}
        </div>
      )}

      {type === 'confidence' && (
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <input
            type="range"
            min="0"
            max="1"
            step="0.05"
            value={clause.min_confidence || 0.5}
            onChange={e =>
              onChange({ ...clause, min_confidence: parseFloat(e.target.value) })
            }
          />
          <span>{((clause.min_confidence || 0.5) * 100).toFixed(0)}%</span>
        </div>
      )}
    </div>
  )
}

// --- Main Query Builder ---

export default function ForensicQueryBuilder({
  onSearch,
}: {
  onSearch: (results: ResultSet) => void
}) {
  const [clauses, setClauses] = useState<QueryNode[]>([
    { type: 'clip', clip_text: '' },
  ])
  const [operator, setOperator] = useState<'AND' | 'OR'>('AND')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const addClause = useCallback(() => {
    setClauses(prev => [...prev, { type: 'clip', clip_text: '' }])
  }, [])

  const updateClause = useCallback((index: number, clause: QueryNode) => {
    setClauses(prev => prev.map((c, i) => (i === index ? clause : c)))
  }, [])

  const removeClause = useCallback((index: number) => {
    setClauses(prev => prev.filter((_, i) => i !== index))
  }, [])

  const buildQuery = useCallback((): QueryNode => {
    const validClauses = clauses.filter(c => {
      if (c.type === 'clip') return !!c.clip_text
      if (c.type === 'object') return !!c.object_class
      if (c.type === 'lpr') return !!c.plate_text
      if (c.type === 'time') return !!c.start || !!c.end
      if (c.type === 'camera') return (c.camera_ids || []).length > 0
      if (c.type === 'time_of_day')
        return !!c.time_of_day_start && !!c.time_of_day_end
      if (c.type === 'day_of_week') return (c.days_of_week || []).length > 0
      if (c.type === 'confidence') return (c.min_confidence || 0) > 0
      return false
    })

    if (validClauses.length === 0) {
      return { type: 'clip', clip_text: '*' }
    }
    if (validClauses.length === 1) {
      return validClauses[0]
    }
    return { op: operator, children: validClauses }
  }, [clauses, operator])

  const executeSearch = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const { apiFetch } = await import('../api/client')
      const res = await apiFetch('/forensic-search', {
        method: 'POST',
        body: JSON.stringify({ query: buildQuery(), limit: 50 }),
      })
      if (!res.ok) {
        const data = await res.json()
        throw new Error(data.error || `HTTP ${res.status}`)
      }
      const data: ResultSet = await res.json()
      onSearch(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Search failed')
    } finally {
      setLoading(false)
    }
  }, [buildQuery, onSearch])

  const loadSample = useCallback(
    async (index: number) => {
      try {
        const { apiFetch } = await import('../api/client')
        const res = await apiFetch('/forensic-search/samples')
        if (!res.ok) return
        const data = await res.json()
        const sample = data.samples?.[index]
        if (!sample?.query) return

        // Flatten sample query children into clauses.
        const q = sample.query as QueryNode
        if (q.children) {
          setClauses(q.children)
          setOperator((q.op as 'AND' | 'OR') || 'AND')
        } else {
          setClauses([q])
        }
      } catch {
        // Ignore sample load errors.
      }
    },
    [],
  )

  return (
    <div style={{ maxWidth: 800 }}>
      <h2>Forensic Search</h2>
      <p style={{ color: '#666', marginBottom: 16 }}>
        Build complex queries combining CLIP text search, object detection,
        license plate recognition, time windows, and camera filters.
      </p>

      <div
        style={{
          display: 'flex',
          gap: 8,
          marginBottom: 16,
          alignItems: 'center',
        }}
      >
        <span>Combine with:</span>
        <select
          value={operator}
          onChange={e => setOperator(e.target.value as 'AND' | 'OR')}
          style={{ fontWeight: 'bold' }}
        >
          <option value="AND">AND (all must match)</option>
          <option value="OR">OR (any can match)</option>
        </select>
        <div style={{ flex: 1 }} />
        <span style={{ color: '#666', fontSize: 12 }}>Samples:</span>
        {[0, 1, 2, 3, 4].map(i => (
          <button
            key={i}
            onClick={() => loadSample(i)}
            style={{ fontSize: 11, padding: '2px 6px' }}
            title={`Load sample query ${i + 1}`}
          >
            #{i + 1}
          </button>
        ))}
      </div>

      {clauses.map((clause, i) => (
        <ClauseEditor
          key={i}
          clause={clause}
          onChange={c => updateClause(i, c)}
          onRemove={() => removeClause(i)}
        />
      ))}

      <div
        style={{
          display: 'flex',
          gap: 8,
          marginTop: 16,
          alignItems: 'center',
        }}
      >
        <button onClick={addClause}>+ Add Clause</button>
        <div style={{ flex: 1 }} />
        <button
          onClick={executeSearch}
          disabled={loading}
          style={{
            padding: '8px 24px',
            fontWeight: 'bold',
            background: '#4a90d9',
            color: '#fff',
            border: 'none',
            borderRadius: 4,
            cursor: loading ? 'wait' : 'pointer',
          }}
        >
          {loading ? 'Searching...' : 'Search'}
        </button>
      </div>

      {error && (
        <div
          style={{
            marginTop: 12,
            padding: 8,
            background: '#fee',
            color: '#c00',
            borderRadius: 4,
          }}
        >
          {error}
        </div>
      )}
    </div>
  )
}

// --- Result List ---

export function ForensicResultList({ data }: { data: ResultSet | null }) {
  if (!data) return null

  return (
    <div style={{ marginTop: 24 }}>
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          marginBottom: 12,
        }}
      >
        <h3>
          Results ({data.total_matches} match
          {data.total_matches !== 1 ? 'es' : ''})
        </h3>
        <span style={{ color: '#666', fontSize: 12, alignSelf: 'center' }}>
          {data.execution_time_ms}ms
        </span>
      </div>

      {data.results.length === 0 && (
        <p style={{ color: '#666' }}>No results found.</p>
      )}

      {data.results.map(result => (
        <div
          key={result.id}
          style={{
            border: '1px solid #ddd',
            borderRadius: 6,
            padding: 12,
            marginBottom: 8,
            display: 'flex',
            gap: 12,
          }}
        >
          {result.thumbnail_path && (
            <img
              src={`/${result.thumbnail_path}`}
              alt="thumbnail"
              style={{
                width: 120,
                height: 80,
                objectFit: 'cover',
                borderRadius: 4,
              }}
            />
          )}
          <div style={{ flex: 1 }}>
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                marginBottom: 4,
              }}
            >
              <strong>
                {result.camera_name || result.camera_id}
              </strong>
              <span
                style={{
                  background: '#e8f0fe',
                  padding: '2px 8px',
                  borderRadius: 10,
                  fontSize: 12,
                }}
              >
                Score: {(result.score * 100).toFixed(0)}%
              </span>
            </div>
            <div style={{ fontSize: 13, color: '#555' }}>
              {new Date(result.timestamp).toLocaleString()}
            </div>
            {result.matched_classes && result.matched_classes.length > 0 && (
              <div style={{ fontSize: 12, marginTop: 4 }}>
                Classes:{' '}
                {result.matched_classes.map(cls => (
                  <span
                    key={cls}
                    style={{
                      background: '#f0f0f0',
                      padding: '1px 6px',
                      borderRadius: 3,
                      marginRight: 4,
                    }}
                  >
                    {cls}
                  </span>
                ))}
              </div>
            )}
            {result.matched_plate && (
              <div style={{ fontSize: 12, marginTop: 4 }}>
                Plate:{' '}
                <span style={{ fontFamily: 'monospace', fontWeight: 'bold' }}>
                  {result.matched_plate}
                </span>
              </div>
            )}
            {result.confidence > 0 && (
              <div style={{ fontSize: 12, color: '#888', marginTop: 2 }}>
                Confidence: {(result.confidence * 100).toFixed(0)}%
                {result.clip_similarity
                  ? ` | CLIP: ${(result.clip_similarity * 100).toFixed(0)}%`
                  : ''}
              </div>
            )}
            {result.snippet_start && result.snippet_end && (
              <div style={{ fontSize: 11, color: '#aaa', marginTop: 2 }}>
                Snippet:{' '}
                {new Date(result.snippet_start).toLocaleTimeString()} -{' '}
                {new Date(result.snippet_end).toLocaleTimeString()}
              </div>
            )}
          </div>
        </div>
      ))}
    </div>
  )
}
