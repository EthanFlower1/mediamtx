import { useState } from 'react'
import ForensicQueryBuilder, {
  ForensicResultList,
} from '../components/ForensicQueryBuilder'

interface ResultSet {
  query: string
  total_matches: number
  results: Array<{
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
  }>
  execution_time_ms: number
}

export default function ForensicSearch() {
  const [results, setResults] = useState<ResultSet | null>(null)

  return (
    <div style={{ padding: 24, maxWidth: 900, margin: '0 auto' }}>
      <ForensicQueryBuilder onSearch={setResults} />
      <ForensicResultList data={results} />
    </div>
  )
}
