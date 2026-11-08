export type ConnectionStatus = 'idle' | 'connecting' | 'connected' | 'offline' | 'degraded' | 'rejected';

export interface RealtimeDiagnosticEvent {
  id: number
  at: string
  kind: 'connecting' | 'opened' | 'snapshot' | 'closed' | 'retry' | 'timeout' | 'error' | 'manual' | 'compression' | 'server_retry'
  message: string
  code?: number
  reason?: string
  wasClean?: boolean
  durationMs?: number
  retryInMs?: number
  intentional?: boolean
  serverInstanceId?: string | null
  connectionId?: string | null
  playerId?: string | null
}

export interface WebSocketProbeResult {
  status: 'ok' | 'not_connected' | 'timeout' | 'closed'
  durationMs: number | null
  serverInstanceId: string | null
  connectionId: string | null
  playerId: string | null
}

export interface LobbyApiProbeData {
  serverInstanceId: string | null
  playerId: string
  revision: number
}

export interface LobbyApiProbeResult {
  status: 'ok' | 'http_error' | 'network_error' | 'timeout'
  httpStatus: number | null
  durationMs: number
  serverInstanceId: string | null
  playerId: string | null
  revision: number | null
}

export interface LobbyConnectionCheck {
  id: number
  at: string
  expectedPlayerId: string | null
  websocketStatus: ConnectionStatus
  api: LobbyApiProbeResult
  websocket: WebSocketProbeResult
  observations: readonly string[]
}
