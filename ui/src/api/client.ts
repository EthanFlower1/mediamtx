let accessToken: string | null = null

export function setAccessToken(token: string | null) {
  accessToken = token
}

export async function apiFetch(path: string, options: RequestInit = {}): Promise<Response> {
  const headers = new Headers(options.headers)
  if (accessToken) {
    headers.set('Authorization', `Bearer ${accessToken}`)
  }
  headers.set('Content-Type', 'application/json')

  let res = await fetch(`/api/nvr${path}`, { ...options, headers, credentials: 'include' })

  if (res.status === 401 && accessToken) {
    // Try refresh
    const refreshRes = await fetch('/api/nvr/auth/refresh', {
      method: 'POST',
      credentials: 'include',
    })
    if (refreshRes.ok) {
      const data = await refreshRes.json()
      accessToken = data.access_token
      headers.set('Authorization', `Bearer ${accessToken}`)
      res = await fetch(`/api/nvr${path}`, { ...options, headers, credentials: 'include' })
    } else {
      accessToken = null
      window.location.href = '/login'
    }
  }

  return res
}
