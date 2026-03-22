let accessToken: string | null = null
let refreshPromise: Promise<boolean> | null = null

const DEFAULT_TIMEOUT_MS = 10000
const MAX_RETRIES = 3
const INITIAL_BACKOFF_MS = 500

export function setAccessToken(token: string | null) {
  accessToken = token
}

export function getAccessToken(): string | null {
  return accessToken
}

/**
 * Performs a single fetch with an AbortController timeout.
 */
async function fetchWithTimeout(
  url: string,
  options: RequestInit,
  timeoutMs: number,
): Promise<Response> {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), timeoutMs)

  try {
    return await fetch(url, { ...options, signal: controller.signal })
  } finally {
    clearTimeout(timer)
  }
}

/**
 * Determines whether a request should be retried based on the error or status.
 */
function shouldRetry(error: unknown, response: Response | null): boolean {
  // Retry on network errors (TypeError from fetch).
  if (error instanceof TypeError) return true
  // Retry on abort (timeout).
  if (error instanceof DOMException && error.name === 'AbortError') return true
  // Retry on 5xx server errors.
  if (response && response.status >= 500) return true
  return false
}

async function refreshToken(): Promise<boolean> {
  if (refreshPromise) return refreshPromise

  refreshPromise = (async () => {
    try {
      const res = await fetchWithTimeout('/api/nvr/auth/refresh', {
        method: 'POST',
        credentials: 'include',
      }, DEFAULT_TIMEOUT_MS)

      if (res.ok) {
        const data = await res.json()
        accessToken = data.access_token
        return true
      }
      accessToken = null
      window.location.href = '/login'
      return false
    } catch {
      accessToken = null
      window.location.href = '/login'
      return false
    } finally {
      refreshPromise = null
    }
  })()

  return refreshPromise
}

export async function apiFetch(path: string, options: RequestInit = {}): Promise<Response> {
  const headers = new Headers(options.headers)
  if (accessToken) {
    headers.set('Authorization', `Bearer ${accessToken}`)
  }
  headers.set('Content-Type', 'application/json')

  const url = `/api/nvr${path}`
  const fetchOptions: RequestInit = { ...options, headers, credentials: 'include' }

  let res: Response | null = null
  let lastError: unknown = null

  for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
    try {
      res = await fetchWithTimeout(url, fetchOptions, DEFAULT_TIMEOUT_MS)
      lastError = null

      // Don't retry on non-5xx responses.
      if (!shouldRetry(null, res)) break
      // On 5xx, retry (unless this is the last attempt).
      if (attempt < MAX_RETRIES) {
        lastError = new Error(`Server error: ${res.status}`)
        res = null
      } else {
        break
      }
    } catch (err) {
      lastError = err
      res = null
      if (!shouldRetry(err, null) || attempt >= MAX_RETRIES) break
    }

    // Exponential backoff before retrying.
    if (attempt < MAX_RETRIES) {
      await new Promise(resolve => setTimeout(resolve, INITIAL_BACKOFF_MS * Math.pow(2, attempt)))
    }
  }

  if (!res) {
    // All retries exhausted -- throw the last error.
    throw lastError ?? new Error('Request failed after retries')
  }

  // Handle 401 with token refresh (no retry loop here, just one attempt).
  if (res.status === 401 && accessToken) {
    const refreshed = await refreshToken()
    if (refreshed) {
      headers.set('Authorization', `Bearer ${accessToken}`)
      res = await fetchWithTimeout(url, { ...options, headers, credentials: 'include' }, DEFAULT_TIMEOUT_MS)
    }
  }

  return res
}
