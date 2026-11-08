import '@angular/compiler';
import { HttpErrorResponse, HttpHeaders } from '@angular/common/http';
import { Injector, runInInjectionContext, signal } from '@angular/core';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { GameStateService } from './game-state.service';
import { RealtimeLobbyService } from './realtime-lobby.service';
import { LobbyDiagnosticsService } from './lobby-diagnostics.service';

vi.mock('./game-state.service', () => ({ GameStateService: class {} }));

describe('LobbyDiagnosticsService', () => {
  const gameState = {
    sessionPlayerId: signal<string | null>('player-one'),
    lobbyCode: signal('ROOM'),
    probeLobbyConnection: vi.fn(),
    reconnectRealtime: vi.fn(),
    privateToken: 'private-token',
  };
  const realtime = {
    compressionEnabled: signal(true),
    compressionActive: signal<boolean | null>(false),
    connectionStatus: signal('connected'),
    serverInstanceId: signal('api-one'),
    connectionId: signal('42'),
    playerId: signal('player-one'),
    lastSnapshotAt: signal('2026-01-01T00:00:00Z'),
    diagnosticEvents: signal([]),
    probeConnection: vi.fn(),
    privateToken: 'private-token',
  };
  let service: LobbyDiagnosticsService;

  beforeEach(() => {
    vi.useFakeTimers();
    gameState.sessionPlayerId.set('player-one');
    realtime.connectionStatus.set('connected');
    gameState.probeLobbyConnection.mockReset().mockResolvedValue({ serverInstanceId: 'api-one', playerId: 'player-one', revision: 7 });
    gameState.reconnectRealtime.mockClear();
    realtime.probeConnection.mockReset().mockResolvedValue({ status: 'ok', durationMs: 12, serverInstanceId: 'api-one', connectionId: '42', playerId: 'player-one' });
    const injector = Injector.create({ providers: [
      { provide: GameStateService, useValue: gameState },
      { provide: RealtimeLobbyService, useValue: realtime },
    ] });
    service = runInInjectionContext(injector, () => new LobbyDiagnosticsService());
  });

  afterEach(() => {
    service.stop();
    vi.useRealTimers();
  });

  it('samples both transports and stops automatically after thirty seconds', async () => {
    service.start();
    await vi.advanceTimersByTimeAsync(0);
    expect(service.latestCheck()).toMatchObject({ api: { status: 'ok' }, websocket: { status: 'ok' } });
    await vi.advanceTimersByTimeAsync(30_000);
    expect(service.running()).toBe(false);
    const count = gameState.probeLobbyConnection.mock.calls.length;
    expect(count).toBeGreaterThan(1);
    await vi.advanceTimersByTimeAsync(30_000);
    expect(gameState.probeLobbyConnection).toHaveBeenCalledTimes(count);
  });

  it('identifies distinct server instances and player identities', async () => {
    gameState.probeLobbyConnection.mockResolvedValue({ serverInstanceId: 'api-two', playerId: 'wrong-player', revision: 7 });
    service.start();
    await vi.advanceTimersByTimeAsync(0);
    expect(service.latestCheck()?.observations.join(' ')).toContain('identité différente');
    expect(service.latestCheck()?.observations.join(' ')).toContain('instances différentes');
  });

  it('compares a failing WebSocket with a successful API', async () => {
    realtime.probeConnection.mockResolvedValue({ status: 'timeout', durationMs: 5000, serverInstanceId: 'old-api', connectionId: '42', playerId: 'player-one' });
    service.start();
    await vi.advanceTimersByTimeAsync(0);
    expect(service.latestCheck()?.observations.join(' ')).toContain('L’API répond, le WebSocket échoue');
    expect(service.latestCheck()?.observations.join(' ')).not.toContain('instances différentes');
  });

  it('does not interpret an idle WebSocket as a transport failure', async () => {
    realtime.connectionStatus.set('idle');
    realtime.probeConnection.mockResolvedValue({ status: 'not_connected', durationMs: null, serverInstanceId: null, connectionId: null, playerId: null });
    service.start();
    await vi.advanceTimersByTimeAsync(0);
    expect(service.latestCheck()?.observations.join(' ')).toContain('Aucun WebSocket actif');
    expect(service.latestCheck()?.observations.join(' ')).not.toContain('proxy');
  });

  it('does not export credentials or raw HTTP errors', async () => {
    gameState.probeLobbyConnection.mockRejectedValue(new HttpErrorResponse({
      status: 401, error: { reconnectToken: 'private-token', answers: 'private-answers' },
      headers: new HttpHeaders({ 'X-Crush-Club-Server': 'api-one', Authorization: 'Bearer private-token' }),
    }));
    service.start();
    await vi.advanceTimersByTimeAsync(0);
    expect(service.latestCheck()?.api.httpStatus).toBe(401);
    expect(service.latestCheck()?.observations.join(' ')).toContain('refuse la session');
    const report = service.buildReport();
    expect(report).not.toContain('private-token');
    expect(report).not.toContain('private-answers');
    expect(JSON.parse(report)).toMatchObject({ expectedPlayerId: 'player-one', websocket: { serverInstanceId: 'api-one' } });
  });

  it('does not overlap slow checks and discards results after stop', async () => {
    let complete: ((value: unknown) => void) | undefined;
    gameState.probeLobbyConnection.mockImplementation(() => new Promise((resolve) => { complete = resolve; }));
    service.start();
    await vi.advanceTimersByTimeAsync(10_000);
    expect(gameState.probeLobbyConnection).toHaveBeenCalledTimes(1);
    service.stop();
    complete?.({ serverInstanceId: 'api-one', playerId: 'player-one', revision: 7 });
    await vi.advanceTimersByTimeAsync(0);
    expect(service.checks()).toHaveLength(0);
  });

  it('requires a player session and delegates manual reconnects', () => {
    gameState.sessionPlayerId.set(null);
    service.start();
    expect(service.running()).toBe(false);
    expect(gameState.probeLobbyConnection).not.toHaveBeenCalled();
    service.reconnect();
    expect(gameState.reconnectRealtime).toHaveBeenCalledTimes(1);
  });
});
