import { HttpErrorResponse } from '@angular/common/http';
import { computed, inject, Injectable, signal } from '@angular/core';
import type { LobbyApiProbeResult, LobbyConnectionCheck } from '../models/realtime-diagnostics.models';
import { GameStateService } from './game-state.service';
import { RealtimeLobbyService } from './realtime-lobby.service';

export function connectionCheckObservations(check: Omit<LobbyConnectionCheck, 'observations'>): readonly string[] {
  const observations: string[] = [];
  if (check.api.status === 'ok' && check.api.playerId !== check.expectedPlayerId) {
    observations.push('L’API renvoie une identité différente de la session enregistrée.');
  }
  if (check.websocket.status === 'ok' && check.websocket.playerId !== check.expectedPlayerId) {
    observations.push('Le WebSocket renvoie une identité différente de la session enregistrée.');
  }
  if (check.api.status === 'ok' && check.websocket.status === 'ok' && check.api.serverInstanceId &&
      check.websocket.serverInstanceId && check.api.serverInstanceId !== check.websocket.serverInstanceId) {
    observations.push('L’API et le WebSocket répondent depuis des instances différentes. Vérifier la répartition entre serveurs.');
  }
  if (check.websocket.status === 'not_connected' && (check.websocketStatus === 'idle' || check.websocketStatus === 'rejected')) {
    observations.push('Aucun WebSocket actif : utiliser « Reconnecter proprement » avant de comparer les transports.');
    if (check.api.status !== 'ok') observations.push('Le test API échoue également. Vérifier la session et la disponibilité du serveur.');
  } else if (check.api.status === 'ok' && check.websocket.status !== 'ok') {
    observations.push('L’API répond, le WebSocket échoue. Vérifier le transport, le proxy et la compression.');
  } else if (check.api.status !== 'ok' && check.websocket.status === 'ok') {
    observations.push('Le WebSocket répond, l’API échoue. Vérifier la session et les lectures en base.');
  } else if (check.api.status !== 'ok' && check.websocket.status !== 'ok') {
    observations.push('Les deux tests échouent. Vérifier le réseau, la session et la disponibilité du serveur.');
  }
  if (check.api.httpStatus === 401) observations.push('L’API refuse la session : rejoindre à nouveau le lobby.');
  if (observations.length === 0) observations.push('Les deux connexions répondent. Consulter l’historique pour les coupures intermittentes.');
  return observations;
}

@Injectable({ providedIn: 'root' })
export class LobbyDiagnosticsService {
  private readonly gameState = inject(GameStateService);
  private readonly realtime = inject(RealtimeLobbyService);
  private readonly checksValue = signal<readonly LobbyConnectionCheck[]>([]);
  private readonly runningValue = signal(false);
  private readonly checkingValue = signal(false);
  private interval: ReturnType<typeof setInterval> | null = null;
  private stopTimer: ReturnType<typeof setTimeout> | null = null;
  private generation = 0;
  private sequence = 0;

  readonly checks = this.checksValue.asReadonly();
  readonly running = this.runningValue.asReadonly();
  readonly checking = this.checkingValue.asReadonly();
  readonly canTest = computed(() => this.gameState.sessionPlayerId() !== null);
  readonly latestCheck = computed(() => this.checksValue().at(-1) ?? null);

  start(): void {
    if (this.runningValue() || !this.canTest()) return;
    const generation = ++this.generation;
    this.runningValue.set(true);
    void this.check(generation);
    this.interval = setInterval(() => { void this.check(generation); }, 5000);
    this.stopTimer = setTimeout(() => this.stop(), 30_000);
  }

  stop(): void {
    this.generation++;
    if (this.interval !== null) clearInterval(this.interval);
    if (this.stopTimer !== null) clearTimeout(this.stopTimer);
    this.interval = null;
    this.stopTimer = null;
    this.runningValue.set(false);
    this.checkingValue.set(false);
  }

  reconnect(): void {
    this.gameState.reconnectRealtime();
  }

  buildReport(): string {
    return JSON.stringify({
      schemaVersion: 1,
      generatedAt: new Date().toISOString(),
      environment: {
        userAgent: typeof navigator === 'undefined' ? null : navigator.userAgent,
        online: typeof navigator === 'undefined' ? null : navigator.onLine,
        visibility: typeof document === 'undefined' ? null : document.visibilityState,
      },
      lobbyCode: this.gameState.lobbyCode(),
      expectedPlayerId: this.gameState.sessionPlayerId(),
      compressionRequested: this.realtime.compressionEnabled(),
      compressionNegotiated: this.realtime.compressionActive(),
      websocket: {
        status: this.realtime.connectionStatus(),
        serverInstanceId: this.realtime.serverInstanceId(),
        connectionId: this.realtime.connectionId(),
        playerId: this.realtime.playerId(),
        lastSnapshotAt: this.realtime.lastSnapshotAt(),
      },
      history: this.realtime.diagnosticEvents(),
      checks: this.checksValue(),
    }, null, 2);
  }

  private async check(generation: number): Promise<void> {
    if (generation !== this.generation || this.checkingValue()) return;
    this.checkingValue.set(true);
    const at = new Date().toISOString();
    const expectedPlayerId = this.gameState.sessionPlayerId();
    const websocketStatus = this.realtime.connectionStatus();
    try {
      const [api, websocket] = await Promise.all([this.probeApi(), this.realtime.probeConnection()]);
      if (generation !== this.generation) return;
      const check = { id: ++this.sequence, at, expectedPlayerId, websocketStatus, api, websocket };
      this.checksValue.update((checks) => [...checks, { ...check, observations: connectionCheckObservations(check) }].slice(-12));
    } finally {
      if (generation === this.generation) this.checkingValue.set(false);
    }
  }

  private async probeApi(): Promise<LobbyApiProbeResult> {
    const startedAt = Date.now();
    try {
      const result = await this.gameState.probeLobbyConnection();
      return { ...result, status: 'ok', httpStatus: 200, durationMs: Date.now() - startedAt };
    } catch (error: unknown) {
      const timedOut = error instanceof Error && error.name === 'TimeoutError';
      const httpStatus = error instanceof HttpErrorResponse && error.status > 0 ? error.status : null;
      return {
        status: timedOut ? 'timeout' : httpStatus === null ? 'network_error' : 'http_error',
        httpStatus, durationMs: Date.now() - startedAt,
        serverInstanceId: error instanceof HttpErrorResponse ? error.headers.get('X-Crush-Club-Server') : null,
        playerId: null, revision: null,
      };
    }
  }
}
