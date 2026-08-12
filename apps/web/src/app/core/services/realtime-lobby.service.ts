import { Injectable, signal } from '@angular/core';
import type { LobbyStateResponse } from '@core/models/game.models';
import { shouldApplyLobbySnapshot } from '@core/guards/lobby-route';

type ConnectionStatus = 'idle' | 'connecting' | 'connected' | 'offline';

interface SnapshotMessage {
  type: 'state.snapshot'
  state: LobbyStateResponse
}

export interface ReactionBroadcastMessage {
  type: 'reaction.broadcast'
  proposalId: string
  emoji: string
  authorPlayerId: string
}

type RealtimeMessage = Partial<SnapshotMessage> | Partial<ReactionBroadcastMessage>;

@Injectable({ providedIn: 'root' })
export class RealtimeLobbyService {
  private readonly stateValue = signal<LobbyStateResponse | null>(null);
  private readonly connectionStatusValue = signal<ConnectionStatus>('idle');
  private readonly reactionEventsValue = signal<readonly ReactionBroadcastMessage[]>([]);
  private socket: WebSocket | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private reconnectAttempt = 0;
  private activeCode = '';
  private activeToken = '';
  private generation = 0;

  readonly state = this.stateValue.asReadonly();
  readonly connectionStatus = this.connectionStatusValue.asReadonly();
  /** Ephemeral Fast Bio emoji reactions pushed live, most recent last. Drain with `clearReactionEvents()`. */
  readonly reactionEvents = this.reactionEventsValue.asReadonly();

  clearReactionEvents(): void {
    this.reactionEventsValue.set([]);
  }

  setState(state: LobbyStateResponse): void {
    const currentState = this.stateValue();
    if (shouldApplyLobbySnapshot(currentState, state)) {
      this.stateValue.set(state);
    }
  }

  connect(code: string, reconnectToken: string): void {
    const normalizedCode = code.toUpperCase();
    if (
      this.activeCode === normalizedCode &&
      this.activeToken === reconnectToken &&
      (this.socket?.readyState === WebSocket.OPEN || this.socket?.readyState === WebSocket.CONNECTING)
    ) {
      return;
    }
    this.disconnect(false);
    this.activeCode = normalizedCode;
    this.activeToken = reconnectToken;
    this.reconnectAttempt = 0;
    this.generation++;
    this.openSocket(this.generation);
  }

  disconnect(clearState = true): void {
    this.generation++;
    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    const socket = this.socket;
    this.socket = null;
    if (socket !== null && socket.readyState < WebSocket.CLOSING) {
      socket.close(1000, 'session closed');
    }
    this.activeCode = '';
    this.activeToken = '';
    this.connectionStatusValue.set('idle');
    this.reactionEventsValue.set([]);
    if (clearState) {
      this.stateValue.set(null);
    }
  }

  private openSocket(generation: number): void {
    if (!this.activeCode || !this.activeToken || generation !== this.generation) {
      return;
    }
    this.connectionStatusValue.set(this.reconnectAttempt === 0 ? 'connecting' : 'offline');
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const socket = new WebSocket(`${protocol}//${window.location.host}/ws/lobbies/${this.activeCode}`);
    this.socket = socket;
    socket.addEventListener('open', () => {
      if (generation !== this.generation) {
        socket.close();
        return;
      }
      socket.send(JSON.stringify({
        type: 'authenticate',
        reconnectToken: this.activeToken,
      }));
    });
    socket.addEventListener('message', (event) => {
      if (generation !== this.generation || typeof event.data !== 'string') {
        return;
      }
      try {
        const message = JSON.parse(event.data) as RealtimeMessage;
        if (message.type === 'reaction.broadcast') {
          if (message.proposalId === undefined || message.emoji === undefined || message.authorPlayerId === undefined) {
            return;
          }
          this.reactionEventsValue.update((events) => [...events, message as ReactionBroadcastMessage]);
          return;
        }
        if (message.type !== 'state.snapshot' || message.state === undefined) {
          return;
        }
        this.reconnectAttempt = 0;
        this.connectionStatusValue.set('connected');
        this.setState(message.state);
      } catch (error: unknown) {
        console.error('Unable to decode lobby snapshot', error);
      }
    });
    socket.addEventListener('close', () => {
      if (generation !== this.generation) {
        return;
      }
      this.socket = null;
      this.connectionStatusValue.set('offline');
      this.scheduleReconnect(generation);
    });
    socket.addEventListener('error', () => {
      if (generation === this.generation) {
        this.connectionStatusValue.set('offline');
      }
    });
  }

  private scheduleReconnect(generation: number): void {
    if (!this.activeCode || !this.activeToken || generation !== this.generation) {
      return;
    }
    const delayList = [1000, 2000, 5000, 10000, 15000] as const;
    const delay = delayList[Math.min(this.reconnectAttempt, delayList.length - 1)];
    this.reconnectAttempt++;
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.openSocket(generation);
    }, delay);
  }
}
