import { HttpClient, HttpHeaders } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import type {
  LobbyResponse,
  LobbyStateResponse,
  PlayerSessionResponse,
  RoundSubmissionInput,
} from '@core/models/game.models';

export interface CreateLobbyInput {
  displayName: string
  maxPlayers: number
  gameConfigId: string
}

@Injectable({
  providedIn: 'root',
})
export class LobbyApiService {
  private readonly http = inject(HttpClient);

  create(input: CreateLobbyInput): Promise<PlayerSessionResponse> {
    return firstValueFrom(this.http.post<PlayerSessionResponse>('/api/v1/lobbies', input));
  }

  join(code: string, displayName: string): Promise<PlayerSessionResponse> {
    return firstValueFrom(
      this.http.post<PlayerSessionResponse>(`/api/v1/lobbies/${code}/players`, { displayName }),
    );
  }

  get(code: string): Promise<LobbyResponse> {
    return firstValueFrom(this.http.get<LobbyResponse>(`/api/v1/lobbies/${code}`));
  }

  getState(code: string, reconnectToken: string): Promise<LobbyStateResponse> {
    return firstValueFrom(
      this.http.get<LobbyStateResponse>(`/api/v1/lobbies/${code}/state`, {
        headers: this.authorizationHeaders(reconnectToken),
      }),
    );
  }

  changeConfig(code: string, reconnectToken: string, gameConfigId: string): Promise<LobbyStateResponse> {
    return firstValueFrom(
      this.http.put<LobbyStateResponse>(
        `/api/v1/lobbies/${code}/game-config`,
        { gameConfigId },
        { headers: this.authorizationHeaders(reconnectToken) },
      ),
    );
  }

  uploadPhotos(
    code: string,
    reconnectToken: string,
    photoList: readonly File[],
  ): Promise<LobbyStateResponse> {
    const formData = new FormData();
    for (const photo of photoList) {
      formData.append('photos', photo, photo.name);
    }
    return firstValueFrom(
      this.http.put<LobbyStateResponse>(`/api/v1/lobbies/${code}/players/me/photos`, formData, {
        headers: this.authorizationHeaders(reconnectToken),
      }),
    );
  }

  getPhoto(code: string, reconnectToken: string, photoId: string): Promise<Blob> {
    return firstValueFrom(
      this.http.get(`/api/v1/lobbies/${code}/photos/${photoId}`, {
        headers: this.authorizationHeaders(reconnectToken),
        responseType: 'blob',
      }),
    );
  }

  start(code: string, reconnectToken: string): Promise<LobbyStateResponse> {
    return firstValueFrom(
      this.http.post<LobbyStateResponse>(`/api/v1/lobbies/${code}/start`, null, {
        headers: this.authorizationHeaders(reconnectToken),
      }),
    );
  }

  submit(
    code: string,
    reconnectToken: string,
    input: RoundSubmissionInput,
  ): Promise<LobbyStateResponse> {
    return firstValueFrom(
      this.http.put<LobbyStateResponse>(`/api/v1/lobbies/${code}/rounds/current/submission`, input, {
        headers: this.authorizationHeaders(reconnectToken),
      }),
    );
  }

  vote(code: string, reconnectToken: string, submissionId: string): Promise<LobbyStateResponse> {
    return firstValueFrom(
      this.http.put<LobbyStateResponse>(
        `/api/v1/lobbies/${code}/rounds/current/vote`,
        { submissionId },
        { headers: this.authorizationHeaders(reconnectToken) },
      ),
    );
  }

  closeCurrentRound(code: string, reconnectToken: string): Promise<LobbyStateResponse> {
    return firstValueFrom(
      this.http.post<LobbyStateResponse>(`/api/v1/lobbies/${code}/rounds/current/next`, null, {
        headers: this.authorizationHeaders(reconnectToken),
      }),
    );
  }

  startNextRound(code: string, reconnectToken: string): Promise<LobbyStateResponse> {
    return firstValueFrom(
      this.http.post<LobbyStateResponse>(`/api/v1/lobbies/${code}/rounds/next/start`, null, {
        headers: this.authorizationHeaders(reconnectToken),
      }),
    );
  }

  excludePlayer(code: string, reconnectToken: string, playerId: string): Promise<LobbyStateResponse> {
    return firstValueFrom(
      this.http.post<LobbyStateResponse>(
        `/api/v1/lobbies/${code}/players/${playerId}/exclude`,
        null,
        { headers: this.authorizationHeaders(reconnectToken) },
      ),
    );
  }

  private authorizationHeaders(reconnectToken: string): HttpHeaders {
    return new HttpHeaders({ Authorization: `Bearer ${reconnectToken}` });
  }
}
