import type { Settings } from './types'

async function req(method: string, url: string, body?: unknown): Promise<any> {
  const res = await fetch(url, {
    method,
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: 'same-origin',
  })
  let data: any = {}
  try {
    data = await res.json()
  } catch {
    /* non-JSON response */
  }
  if (!res.ok) {
    throw new Error(data?.error || `HTTP ${res.status}`)
  }
  return data
}

export const api = {
  bootstrap: () => req('GET', '/api/bootstrap'),
  setup: (password: string) => req('POST', '/api/setup', { password }),
  login: (password: string) => req('POST', '/api/login', { password }),
  logout: () => req('POST', '/api/logout'),

  getSettings: (): Promise<Settings> => req('GET', '/api/settings'),
  saveSettings: (s: Settings) => req('PUT', '/api/settings', s),
  testProvider: (p: { id?: string; base_url?: string; api_key?: string }) =>
    req('POST', '/api/settings/test', p),

  conversations: () => req('GET', '/api/conversations'),
  deleteConversation: (id: string) => req('DELETE', `/api/conversations/${id}`),
  messages: (id: string) => req('GET', `/api/conversations/${id}/messages`),

  chat: (conversationId: string | null, message: string) =>
    req('POST', '/api/chat', { conversation_id: conversationId, message }),

  activeRuns: (): Promise<any[]> => req('GET', '/api/runs/active'),
  approve: (runId: string, approved: boolean) =>
    req('POST', `/api/runs/${runId}/approve`, { approved }),
  cancelRun: (runId: string) => req('POST', `/api/runs/${runId}/cancel`),

  backups: () => req('GET', '/api/backups'),
  restoreBackup: (name: string) => req('POST', '/api/backups/restore', { name }),

  changePassword: (current: string, next: string) =>
    req('POST', '/api/password', { current, new: next }),
}
