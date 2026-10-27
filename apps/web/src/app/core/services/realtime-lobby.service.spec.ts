import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { LobbyStateResponse } from '../models/game.models';
import { isAppleMobileDevice, RealtimeLobbyService } from './realtime-lobby.service';

class TestWebSocket extends EventTarget {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;
  static instances: TestWebSocket[] = [];

  readyState = TestWebSocket.CONNECTING;
  extensions = '';
  readonly send = vi.fn();
  readonly close = vi.fn(() => {
    this.readyState = TestWebSocket.CLOSED;
    this.dispatchEvent(new Event('close'));
  });

  constructor(readonly url: string) {
    super();
    TestWebSocket.instances.push(this);
  }

  open(extensions = ''): void {
    this.extensions = extensions;
    this.readyState = TestWebSocket.OPEN;
    this.dispatchEvent(new Event('open'));
  }

  snapshot(state: LobbyStateResponse): void {
    this.message({ type: 'state.snapshot', state });
  }

  message(message: unknown): void {
    this.dispatchEvent(Object.assign(new Event('message'), { data: JSON.stringify(message) }));
  }

  serverClose(code = 1006, reason = '', wasClean = false): void {
    this.readyState = TestWebSocket.CLOSED;
    this.dispatchEvent(Object.assign(new Event('close'), { code, reason, wasClean }));
  }
}

const state: LobbyStateResponse = {
  code: 'ROOM',
  revision: 1,
  serverTime: '2026-01-01T00:00:00Z',
  status: 'waiting_for_players',
  mode: 'fast_bio',
  maxPlayers: null,
  currentPlayerId: 'player-one',
  players: [],
  gameConfig: { id: 'config', name: 'Classique', version: 1, questionCount: 0 },
  questionnaire: { sourceConfigId: 'config', sourceVersion: 1, name: 'Classique', questions: [], profileFields: [] },
};

const desktopUserAgent = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36';
const iPhoneUserAgent = 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/130.0 Mobile/15E148 Safari/604.1';
const macUserAgent = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15';

describe('isAppleMobileDevice', () => {
  it('detects iPhone and iPadOS but not Mac or desktop browsers', () => {
    expect(isAppleMobileDevice({ userAgent: iPhoneUserAgent, maxTouchPoints: 5 })).toBe(true);
    expect(isAppleMobileDevice({ userAgent: macUserAgent, maxTouchPoints: 5 })).toBe(true);
    expect(isAppleMobileDevice({ userAgent: macUserAgent, maxTouchPoints: 0 })).toBe(false);
    expect(isAppleMobileDevice({ userAgent: desktopUserAgent, maxTouchPoints: 0 })).toBe(false);
  });
});

describe('RealtimeLobbyService compression preference', () => {
  let stored: Map<string, string>;

  beforeEach(() => {
    vi.useFakeTimers();
    stored = new Map();
    TestWebSocket.instances = [];
    vi.stubGlobal('WebSocket', TestWebSocket);
    vi.stubGlobal('window', { location: { protocol: 'https:', host: 'crush.test' } });
    vi.stubGlobal('navigator', { userAgent: desktopUserAgent, maxTouchPoints: 0 });
    vi.stubGlobal('localStorage', {
      getItem: (key: string) => stored.get(key) ?? null,
      setItem: (key: string, value: string) => stored.set(key, value),
    });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('enables compression by default outside iOS and authenticates outside the URL', () => {
    const service = new RealtimeLobbyService();
    service.connect('room', 'private-token');
    const socket = TestWebSocket.instances[0];
    expect(socket.url).toBe('wss://crush.test/ws/lobbies/ROOM?compression=on');
    socket.open('permessage-deflate');
    expect(service.compressionActive()).toBe(true);
    expect(socket.send).toHaveBeenCalledWith(JSON.stringify({ type: 'authenticate', reconnectToken: 'private-token' }));
    service.disconnect();
  });

  it('disables compression by default on iPhone and iPad', () => {
    for (const device of [{ userAgent: iPhoneUserAgent, maxTouchPoints: 5 }, { userAgent: macUserAgent, maxTouchPoints: 5 }]) {
      TestWebSocket.instances = [];
      vi.stubGlobal('navigator', device);
      const service = new RealtimeLobbyService();
      expect(service.compressionEnabled()).toBe(false);
      service.connect('ROOM', 'private-token');
      expect(TestWebSocket.instances[0].url).toContain('?compression=off');
      service.disconnect();
    }
  });

  it('lets an explicit choice override the iOS default', () => {
    vi.stubGlobal('navigator', { userAgent: iPhoneUserAgent, maxTouchPoints: 5 });
    stored.set('crush-club-websocket-compression-v1', 'on');
    const service = new RealtimeLobbyService();
    expect(service.compressionEnabled()).toBe(true);
    service.connect('ROOM', 'private-token');
    expect(TestWebSocket.instances[0].url).toContain('?compression=on');
    service.disconnect();
  });

  it('reconnects once when toggled, preserves the lobby, and ignores the old socket closing', () => {
    const service = new RealtimeLobbyService();
    service.connect('ROOM', 'private-token');
    const previous = TestWebSocket.instances[0];
    previous.open('permessage-deflate');
    previous.snapshot(state);
    expect(service.connectionStatus()).toBe('connected');

    service.setCompressionEnabled(false);
    expect(previous.close).toHaveBeenCalledTimes(1);
    expect(TestWebSocket.instances).toHaveLength(2);
    const replacement = TestWebSocket.instances[1];
    expect(replacement.url).toBe('wss://crush.test/ws/lobbies/ROOM?compression=off');
    expect(service.state()).toEqual(state);
    expect(service.compressionActive()).toBeNull();
    previous.dispatchEvent(new Event('close'));
    vi.advanceTimersByTime(2000);
    expect(TestWebSocket.instances).toHaveLength(2);

    replacement.open();
    replacement.snapshot(state);
    expect(service.compressionActive()).toBe(false);
    expect(service.connectionStatus()).toBe('connected');
    service.setCompressionEnabled(false);
    expect(TestWebSocket.instances).toHaveLength(2);
    service.disconnect();
  });

  it('restores the preference after reload and uses it for automatic reconnects', () => {
    const original = new RealtimeLobbyService();
    original.setCompressionEnabled(false);
    expect(TestWebSocket.instances).toHaveLength(0);
    expect(original.compressionPreferenceSaved()).toBe(true);

    const restored = new RealtimeLobbyService();
    expect(restored.compressionEnabled()).toBe(false);
    restored.connect('ROOM', 'private-token');
    TestWebSocket.instances[0].close();
    vi.advanceTimersByTime(1000);
    expect(TestWebSocket.instances[1].url).toContain('?compression=off');
    restored.disconnect();
  });

  it('reports inactive compression when the server does not negotiate it', () => {
    const service = new RealtimeLobbyService();
    service.connect('ROOM', 'private-token');
    TestWebSocket.instances[0].open();
    expect(service.compressionEnabled()).toBe(true);
    expect(service.compressionActive()).toBe(false);
    service.disconnect();
  });

  it('can change compression even when browser storage is unavailable', () => {
    vi.stubGlobal('localStorage', {
      getItem: () => { throw new Error('Storage blocked'); },
      setItem: () => { throw new Error('Storage blocked'); },
    });
    const service = new RealtimeLobbyService();
    expect(service.compressionEnabled()).toBe(true);
    service.setCompressionEnabled(false);
    expect(service.compressionEnabled()).toBe(false);
    expect(service.compressionPreferenceSaved()).toBe(false);
    service.connect('ROOM', 'private-token');
    expect(TestWebSocket.instances[0].url).toContain('?compression=off');
    service.disconnect();
  });

  it('backs off across brief successful connections and respects an already scheduled retry', () => {
    const service = new RealtimeLobbyService();
    service.connect('ROOM', 'private-token');
    for (const delay of [1000, 2000, 5000]) {
      const socket = TestWebSocket.instances.at(-1)!;
      socket.open();
      socket.snapshot(state);
      socket.serverClose();
      const count = TestWebSocket.instances.length;
      service.connect('ROOM', 'private-token');
      vi.advanceTimersByTime(delay - 1);
      expect(TestWebSocket.instances).toHaveLength(count);
      vi.advanceTimersByTime(1);
      expect(TestWebSocket.instances).toHaveLength(count + 1);
    }
    expect(service.state()).toEqual(state);
    service.disconnect();
  });

  it('resets the retry delay only after twenty seconds of stable connection', () => {
    const service = new RealtimeLobbyService();
    service.connect('ROOM', 'private-token');
    TestWebSocket.instances[0].serverClose();
    vi.advanceTimersByTime(1000);
    const socket = TestWebSocket.instances[1];
    socket.open();
    socket.snapshot(state);
    vi.advanceTimersByTime(20_000);
    socket.serverClose();
    vi.advanceTimersByTime(1000);
    expect(TestWebSocket.instances).toHaveLength(3);
    service.disconnect();
  });

  it('stops retrying rejected sessions and redacts tokens in close reasons', () => {
    for (const code of [4401, 4404]) {
      TestWebSocket.instances = [];
      const service = new RealtimeLobbyService();
      service.connect('ROOM', 'private-token');
      TestWebSocket.instances[0].serverClose(code, 'invalid private-token');
      vi.advanceTimersByTime(60_000);
      service.connect('ROOM', 'private-token');
      expect(service.connectionStatus()).toBe('rejected');
      expect(TestWebSocket.instances).toHaveLength(1);
      expect(JSON.stringify(service.diagnosticEvents())).not.toContain('private-token');
      service.connect('ROOM', 'private-token', true);
      expect(TestWebSocket.instances).toHaveLength(2);
      service.disconnect();
    }
  });

  it('retries temporary server refusals, including a late authentication', () => {
    for (const code of [1008, 1013]) {
      TestWebSocket.instances = [];
      const service = new RealtimeLobbyService();
      service.connect('ROOM', 'private-token');
      TestWebSocket.instances[0].serverClose(code, 'authentication required');
      expect(service.connectionStatus()).toBe('offline');
      vi.advanceTimersByTime(1000);
      expect(TestWebSocket.instances).toHaveLength(2);
      service.disconnect();
    }
  });

  it('recovers from a stuck connection even if close never fires', () => {
    const service = new RealtimeLobbyService();
    service.connect('ROOM', 'private-token');
    const stalled = TestWebSocket.instances[0];
    stalled.close.mockImplementation(() => { stalled.readyState = TestWebSocket.CLOSING; });
    vi.advanceTimersByTime(20_000);
    expect(stalled.close).toHaveBeenCalledWith(4000, 'connection timeout');
    vi.advanceTimersByTime(1000);
    expect(TestWebSocket.instances).toHaveLength(2);
    TestWebSocket.instances[1].open();
    TestWebSocket.instances[1].snapshot(state);
    stalled.snapshot({ ...state, revision: 99, currentPlayerId: 'wrong-player' });
    expect(service.state()?.currentPlayerId).toBe('player-one');
    expect(service.connectionStatus()).toBe('connected');
    service.disconnect();
  });

  it('keeps a degraded connection open and probes it without changing lobby state', async () => {
    const service = new RealtimeLobbyService();
    service.connect('ROOM', 'private-token');
    const socket = TestWebSocket.instances[0];
    socket.open();
    socket.message({ type: 'state.unavailable', retryInMs: 1000, serverInstanceId: 'api-one', connectionId: '42', playerId: 'player-one' });
    vi.advanceTimersByTime(30_000);
    expect(socket.close).not.toHaveBeenCalled();
    expect(service.connectionStatus()).toBe('degraded');
    const result = service.probeConnection();
    const ping = JSON.parse(socket.send.mock.calls.at(-1)![0] as string) as { id: string };
    socket.message({ type: 'diagnostic.pong', id: ping.id, serverInstanceId: 'api-one', connectionId: '42', playerId: 'player-one' });
    expect(await result).toMatchObject({ status: 'ok', serverInstanceId: 'api-one', connectionId: '42' });
    expect(service.state()).toBeNull();
    socket.snapshot(state);
    expect(service.connectionStatus()).toBe('connected');
    service.disconnect();
  });

  it('finishes pending probes when a connection closes', async () => {
    const service = new RealtimeLobbyService();
    service.connect('ROOM', 'private-token');
    TestWebSocket.instances[0].open();
    const result = service.probeConnection();
    TestWebSocket.instances[0].serverClose();
    expect(await result).toMatchObject({ status: 'closed' });
    service.disconnect();
  });

  it('reports a probe timeout and bounds the event history', async () => {
    const service = new RealtimeLobbyService();
    service.connect('ROOM', 'private-token');
    const socket = TestWebSocket.instances[0];
    socket.open();
    socket.snapshot(state);
    const probe = service.probeConnection();
    vi.advanceTimersByTime(5000);
    expect(await probe).toMatchObject({ status: 'timeout', durationMs: 5000 });
    for (let index = 0; index < 60; index++) socket.dispatchEvent(new Event('error'));
    expect(service.diagnosticEvents()).toHaveLength(50);
    service.disconnect();
  });
});
