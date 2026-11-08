import { Injectable, signal } from '@angular/core';
import type { LobbyStateResponse } from '../models/game.models';
import type { ConnectionStatus, RealtimeDiagnosticEvent, WebSocketProbeResult } from '../models/realtime-diagnostics.models';
import { shouldApplyLobbySnapshot } from '../guards/lobby-route';

interface ConnectionMetadata {
  serverInstanceId?: string
  connectionId?: string
  playerId?: string
}

interface SnapshotMessage extends ConnectionMetadata {
  type: 'state.snapshot'
  state: LobbyStateResponse
}

interface StateUnavailableMessage extends ConnectionMetadata {
  type: 'state.unavailable'
  retryInMs: number
}

interface DiagnosticPongMessage extends ConnectionMetadata {
  type: 'diagnostic.pong'
  id: string
}

export interface ReactionBroadcastMessage {
  type: 'reaction.broadcast'
  proposalId: string
  emoji: string
  authorPlayerId: string
}

type RealtimeMessage = Partial<SnapshotMessage> | Partial<ReactionBroadcastMessage> |
  Partial<StateUnavailableMessage> | Partial<DiagnosticPongMessage>;

const compressionStorageKey = 'crush-club-websocket-compression-v1';
const connectionTimeoutMs = 20_000;
const stableConnectionMs = 20_000;

/** iPhone, iPod or iPad; iPadOS presents itself as a Mac with a touch screen. */
export function isAppleMobileDevice(device: { userAgent: string, maxTouchPoints?: number }): boolean {
  return /iPhone|iPad|iPod/.test(device.userAgent) || (/Macintosh/.test(device.userAgent) && (device.maxTouchPoints ?? 0) > 1);
}

@Injectable({ providedIn: 'root' })
export class RealtimeLobbyService {
  private readonly stateValue = signal<LobbyStateResponse | null>(null);
  private readonly connectionStatusValue = signal<ConnectionStatus>('idle');
  private readonly reactionEventsValue = signal<readonly ReactionBroadcastMessage[]>([]);
  private readonly compressionEnabledValue = signal(this.readCompressionPreference());
  private readonly compressionActiveValue = signal<boolean | null>(null);
  private readonly compressionPreferenceSavedValue = signal(true);
  private readonly diagnosticEventsValue = signal<readonly RealtimeDiagnosticEvent[]>([]);
  private readonly serverInstanceIdValue = signal<string | null>(null);
  private readonly connectionIdValue = signal<string | null>(null);
  private readonly playerIdValue = signal<string | null>(null);
  private readonly lastSnapshotAtValue = signal<string | null>(null);
  private socket: WebSocket | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private connectionTimer: ReturnType<typeof setTimeout> | null = null;
  private stableTimer: ReturnType<typeof setTimeout> | null = null;
  private reconnectAttempt = 0;
  private activeCode = '';
  private activeToken = '';
  private generation = 0;
  private eventSequence = 0;
  private probeSequence = 0;
  private readonly pendingProbes = new Map<string, {
    startedAt: number
    timer: ReturnType<typeof setTimeout>
    resolve: (result: WebSocketProbeResult) => void
  }>();

  readonly state = this.stateValue.asReadonly();
  readonly connectionStatus = this.connectionStatusValue.asReadonly();
  readonly compressionEnabled = this.compressionEnabledValue.asReadonly();
  readonly compressionActive = this.compressionActiveValue.asReadonly();
  readonly compressionPreferenceSaved = this.compressionPreferenceSavedValue.asReadonly();
  readonly diagnosticEvents = this.diagnosticEventsValue.asReadonly();
  readonly serverInstanceId = this.serverInstanceIdValue.asReadonly();
  readonly connectionId = this.connectionIdValue.asReadonly();
  readonly playerId = this.playerIdValue.asReadonly();
  readonly lastSnapshotAt = this.lastSnapshotAtValue.asReadonly();
  readonly reactionEvents = this.reactionEventsValue.asReadonly();

  clearReactionEvents(): void {
    this.reactionEventsValue.set([]);
  }

  setCompressionEnabled(enabled: boolean): void {
    if (enabled === this.compressionEnabledValue()) return;
    this.compressionEnabledValue.set(enabled);
    try {
      localStorage.setItem(compressionStorageKey, enabled ? 'on' : 'off');
      this.compressionPreferenceSavedValue.set(true);
    } catch {
      this.compressionPreferenceSavedValue.set(false);
    }
    this.record('compression', enabled ? 'Compression autorisée' : 'Compression désactivée');
    if (this.activeCode && this.activeToken) this.connect(this.activeCode, this.activeToken, true);
  }

  setState(state: LobbyStateResponse): void {
    if (shouldApplyLobbySnapshot(this.stateValue(), state)) this.stateValue.set(state);
  }

  connect(code: string, reconnectToken: string, forceReconnect = false): void {
    const normalizedCode = code.toUpperCase();
    const sameSession = this.activeCode === normalizedCode && this.activeToken === reconnectToken;
    if (!forceReconnect && sameSession && (
      this.socket?.readyState === WebSocket.OPEN || this.socket?.readyState === WebSocket.CONNECTING ||
      this.reconnectTimer !== null || this.connectionStatusValue() === 'rejected'
    )) return;
    if (forceReconnect) this.record('manual', 'Reconnexion demandée');
    this.disconnect(false);
    this.activeCode = normalizedCode;
    this.activeToken = reconnectToken;
    this.reconnectAttempt = 0;
    this.openSocket(this.generation);
  }

  disconnect(clearState = true): void {
    this.generation++;
    if (this.reconnectTimer !== null) clearTimeout(this.reconnectTimer);
    this.reconnectTimer = null;
    this.clearSocketTimers();
    this.finishProbes();
    const socket = this.socket;
    this.socket = null;
    if (socket !== null && socket.readyState < WebSocket.CLOSING) {
      this.record('closed', 'Connexion fermée volontairement', { intentional: true, code: 1000 });
      socket.close(1000, 'session closed');
    }
    this.activeCode = '';
    this.activeToken = '';
    this.connectionStatusValue.set('idle');
    this.compressionActiveValue.set(null);
    this.reactionEventsValue.set([]);
    if (clearState) {
      this.stateValue.set(null);
      this.serverInstanceIdValue.set(null);
      this.connectionIdValue.set(null);
      this.playerIdValue.set(null);
      this.lastSnapshotAtValue.set(null);
    }
  }

  probeConnection(): Promise<WebSocketProbeResult> {
    const socket = this.socket;
    if (socket?.readyState !== WebSocket.OPEN) return Promise.resolve(this.probeResult('not_connected', null));
    const id = 'probe-' + ++this.probeSequence;
    return new Promise((resolve) => {
      const startedAt = Date.now();
      const timer = setTimeout(() => {
        this.pendingProbes.delete(id);
        resolve(this.probeResult('timeout', Date.now() - startedAt));
      }, 5000);
      this.pendingProbes.set(id, { startedAt, timer, resolve });
      try {
        socket.send(JSON.stringify({ type: 'diagnostic.ping', id }));
      } catch {
        clearTimeout(timer);
        this.pendingProbes.delete(id);
        resolve(this.probeResult('closed', Date.now() - startedAt));
      }
    });
  }

  private openSocket(generation: number): void {
    if (!this.activeCode || !this.activeToken || generation !== this.generation) return;
    this.connectionStatusValue.set(this.reconnectAttempt === 0 ? 'connecting' : 'offline');
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const compression = this.compressionEnabledValue() ? 'on' : 'off';
    const socket = new WebSocket(protocol + '//' + window.location.host + '/ws/lobbies/' + this.activeCode + '?compression=' + compression);
    this.socket = socket;
    let connectedAt: number | null = null;
    let receivedSnapshot = false;
    const isCurrent = (): boolean => generation === this.generation && this.socket === socket;
    this.record('connecting', 'Ouverture de la connexion');
    this.connectionTimer = setTimeout(() => {
      if (!isCurrent()) return;
      this.socket = null;
      this.clearSocketTimers();
      this.finishProbes();
      this.compressionActiveValue.set(null);
      this.connectionStatusValue.set('offline');
      this.record('timeout', 'Délai de connexion ou d’authentification dépassé', { code: 4000 });
      if (socket.readyState < WebSocket.CLOSING) socket.close(4000, 'connection timeout');
      this.scheduleReconnect(generation);
    }, connectionTimeoutMs);
    socket.addEventListener('open', () => {
      if (!isCurrent()) { socket.close(); return; }
      connectedAt = Date.now();
      this.compressionActiveValue.set(socket.extensions.includes('permessage-deflate'));
      this.record('opened', 'WebSocket ouvert');
      socket.send(JSON.stringify({ type: 'authenticate', reconnectToken: this.activeToken }));
    });
    socket.addEventListener('message', (event) => {
      if (!isCurrent() || typeof event.data !== 'string') return;
      try {
        const message = JSON.parse(event.data) as RealtimeMessage;
        if (message.type === 'diagnostic.pong') {
          if (typeof message.id !== 'string') return;
          this.applyMetadata(message);
          const probe = this.pendingProbes.get(message.id);
          if (probe) {
            clearTimeout(probe.timer);
            this.pendingProbes.delete(message.id);
            probe.resolve(this.probeResult('ok', Date.now() - probe.startedAt));
          }
          return;
        }
        if (message.type === 'state.unavailable') {
          this.applyMetadata(message);
          this.clearSocketTimers();
          this.connectionStatusValue.set('degraded');
          this.record('server_retry', 'Le serveur réessaie de charger l’état du lobby', { retryInMs: message.retryInMs });
          return;
        }
        if (message.type === 'reaction.broadcast') {
          if (message.proposalId === undefined || message.emoji === undefined || message.authorPlayerId === undefined) return;
          this.reactionEventsValue.update((events) => [...events, message as ReactionBroadcastMessage]);
          return;
        }
        if (message.type !== 'state.snapshot' || message.state === undefined) return;
        this.applyMetadata(message);
        this.playerIdValue.set(message.state.currentPlayerId);
        this.lastSnapshotAtValue.set(new Date().toISOString());
        if (this.connectionTimer !== null) clearTimeout(this.connectionTimer);
        this.connectionTimer = null;
        if (this.stableTimer === null) {
          // A brief successful reconnect must not restart a rapid reconnect loop.
          this.stableTimer = setTimeout(() => {
            if (isCurrent()) this.reconnectAttempt = 0;
          }, stableConnectionMs);
        }
        if (!receivedSnapshot) this.record('snapshot', 'Premier état du lobby reçu');
        receivedSnapshot = true;
        this.connectionStatusValue.set('connected');
        this.setState(message.state);
      } catch {
        this.record('error', 'Message du serveur illisible');
      }
    });
    socket.addEventListener('close', (event) => {
      if (!isCurrent()) return;
      this.socket = null;
      this.clearSocketTimers();
      this.finishProbes();
      this.compressionActiveValue.set(null);
      // Only explicit session/lobby refusals stop retries; 1008 can also mean a late authentication message.
      const rejected = event.code === 4401 || event.code === 4404;
      this.connectionStatusValue.set(rejected ? 'rejected' : 'offline');
      const reason = (event.reason ?? '').split(this.activeToken).join('[masqué]').slice(0, 160);
      this.record('closed', rejected ? 'Session refusée : reconnexion automatique arrêtée' : 'Connexion interrompue', {
        code: event.code, reason, wasClean: event.wasClean,
        durationMs: connectedAt === null ? undefined : Date.now() - connectedAt,
      });
      if (!rejected) this.scheduleReconnect(generation);
    });
    socket.addEventListener('error', () => {
      if (isCurrent()) this.record('error', 'Erreur de transport WebSocket');
    });
  }

  private scheduleReconnect(generation: number): void {
    if (!this.activeCode || !this.activeToken || generation !== this.generation || this.reconnectTimer !== null) return;
    const delays = [1000, 2000, 5000, 10000, 15000] as const;
    const delay = delays[Math.min(this.reconnectAttempt++, delays.length - 1)];
    this.record('retry', 'Nouvelle tentative programmée', { retryInMs: delay });
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.openSocket(generation);
    }, delay);
  }

  private clearSocketTimers(): void {
    if (this.connectionTimer !== null) clearTimeout(this.connectionTimer);
    if (this.stableTimer !== null) clearTimeout(this.stableTimer);
    this.connectionTimer = null;
    this.stableTimer = null;
  }

  private applyMetadata(message: ConnectionMetadata): void {
    if (typeof message.serverInstanceId === 'string') this.serverInstanceIdValue.set(message.serverInstanceId.slice(0, 80));
    if (typeof message.connectionId === 'string') this.connectionIdValue.set(message.connectionId.slice(0, 80));
    if (typeof message.playerId === 'string') this.playerIdValue.set(message.playerId.slice(0, 80));
  }

  private record(kind: RealtimeDiagnosticEvent['kind'], message: string, details: Partial<RealtimeDiagnosticEvent> = {}): void {
    const event: RealtimeDiagnosticEvent = {
      ...details, id: ++this.eventSequence, at: new Date().toISOString(), kind, message,
      serverInstanceId: this.serverInstanceIdValue(), connectionId: this.connectionIdValue(), playerId: this.playerIdValue(),
    };
    this.diagnosticEventsValue.update((events) => [...events, event].slice(-50));
  }

  private probeResult(status: WebSocketProbeResult['status'], durationMs: number | null): WebSocketProbeResult {
    return { status, durationMs, serverInstanceId: this.serverInstanceIdValue(), connectionId: this.connectionIdValue(), playerId: this.playerIdValue() };
  }

  private finishProbes(): void {
    for (const probe of this.pendingProbes.values()) {
      clearTimeout(probe.timer);
      probe.resolve(this.probeResult('closed', Date.now() - probe.startedAt));
    }
    this.pendingProbes.clear();
  }

  private readCompressionPreference(): boolean {
    let stored: string | null = null;
    try { stored = localStorage.getItem(compressionStorageKey); }
    catch { /* Storage blocked: fall back to the device default. */ }
    if (stored === 'on' || stored === 'off') return stored === 'on';
    // iOS WebKit drops the fragmented frames that compressed messages are sent in.
    return typeof navigator === 'undefined' || !isAppleMobileDevice(navigator);
  }
}
