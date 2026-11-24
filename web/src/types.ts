// API types mirroring the Go backend JSON.

export interface Provider {
  id: string
  name: string
  base_url: string
  api_key: string
  model: string
}

export interface Settings {
  enabled: boolean
  port: number
  listen: string
  confirm_write: boolean
  default_provider: string
  providers: Provider[] | null
}

export interface PendingAction {
  tool_call_id: string
  tool: string
  args: unknown
  level: string
  description: string
  configs?: string[] | null
}

export interface RunSnapshot {
  id: string
  conversation_id: string
  state: string
  error?: string
  pending?: PendingAction | null
}

export interface Conversation {
  id: string
  title: string
  created: number
  updated: number
}

export interface Entry {
  t: string // user | assistant | tool | approval | error
  text?: string
  tool?: string
  desc?: string
  result?: string
  level?: string
  approved?: boolean | null
  ts: number
}

export interface RunEvent {
  seq: number
  type: string // state | text_delta | tool_start | tool_result | approval_request | error | done
  state?: string
  text?: string
  tool?: string
  desc?: string
  level?: string
  result?: string
  ok?: boolean
  error?: string
  pending?: PendingAction | null
}

export interface BackupInfo {
  name: string
  config: string
  size: number
  time: number
}
