import { HttpClient, HttpHeaders } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { firstValueFrom, timeout } from 'rxjs';
import type {
  LobbyMode,
  LobbyResponse,
  LobbyStateResponse,
  PlayerSessionResponse,
  RoundSubmissionInput,
} from '@core/models/game.models';

export interface CreateLobbyInput {
  displayName: string
  mode: LobbyMode
  maxPlayers?: number
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

  addPhoto(code: string, reconnectToken: string, photo: File): Promise<LobbyStateResponse> {
    return firstValueFrom(
      this.http.post<LobbyStateResponse>(
        `/api/v1/lobbies/${code}/players/me/photos`,
        this.singlePhotoFormData(photo),
        { headers: this.authorizationHeaders(reconnectToken) },
      ),
    );
  }

  replacePhoto(
    code: string,
    reconnectToken: string,
    position: number,
    photo: File,
  ): Promise<LobbyStateResponse> {
    return firstValueFrom(
      this.http.put<LobbyStateResponse>(
        `/api/v1/lobbies/${code}/players/me/photos/${position}`,
        this.singlePhotoFormData(photo),
        { headers: this.authorizationHeaders(reconnectToken) },
      ),
    );
  }

  deletePhoto(code: string, reconnectToken: string, position: number): Promise<LobbyStateResponse> {
    return firstValueFrom(
      this.http.delete<LobbyStateResponse>(
        `/api/v1/lobbies/${code}/players/me/photos/${position}`,
        { headers: this.authorizationHeaders(reconnectToken) },
      ),
    );
  }

  completePhotos(code: string, reconnectToken: string): Promise<LobbyStateResponse> {
    return firstValueFrom(
      this.http.post<LobbyStateResponse>(
        `/api/v1/lobbies/${code}/players/me/photos/complete`,
        null,
        { headers: this.authorizationHeaders(reconnectToken) },
      ),
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

  leave(code: string, reconnectToken: string): Promise<void> {
    return firstValueFrom(
      this.http.post<void>(`/api/v1/lobbies/${code}/players/me/leave`, null, {
        headers: this.authorizationHeaders(reconnectToken),
      }).pipe(timeout(3000)),
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

  startFastBio(code: string, reconnectToken: string): Promise<LobbyStateResponse> {
    return firstValueFrom(
      this.http.post<LobbyStateResponse>(`/api/v1/lobbies/${code}/fast-bio/start`, null, {
        headers: this.authorizationHeaders(reconnectToken),
      }),
    );
  }

  submitFastBioTheme(code: string, reconnectToken: string, theme: string): Promise<LobbyStateResponse> {
    return firstValueFrom(
      this.http.post<LobbyStateResponse>(
        `/api/v1/lobbies/${code}/fast-bio/themes`,
        { theme },
        { headers: this.authorizationHeaders(reconnectToken) },
      ),
    );
  }

  rankFastBioThemes(code: string, reconnectToken: string, ranking: readonly string[]): Promise<LobbyStateResponse> {
    return firstValueFrom(
      this.http.post<LobbyStateResponse>(
        `/api/v1/lobbies/${code}/fast-bio/themes/rank`,
        { ranking },
        { headers: this.authorizationHeaders(reconnectToken) },
      ),
    );
  }

  submitFastBioProposal(
    code: string,
    reconnectToken: string,
    photo: File,
    bio: string,
  ): Promise<LobbyStateResponse> {
    const formData = new FormData();
    formData.append('photo', photo, photo.name);
    formData.append('bio', bio);
    return firstValueFrom(
      this.http.post<LobbyStateResponse>(`/api/v1/lobbies/${code}/fast-bio/proposal`, formData, {
        headers: this.authorizationHeaders(reconnectToken),
      }),
    );
  }

  getFastBioProposalPhoto(code: string, reconnectToken: string, proposalId: string): Promise<Blob> {
    return firstValueFrom(
      this.http.get(`/api/v1/lobbies/${code}/fast-bio/photos/${proposalId}`, {
        headers: this.authorizationHeaders(reconnectToken),
        responseType: 'blob',
      }),
    );
  }

  advanceFastBioReview(
    code: string,
    reconnectToken: string,
    direction: 'next' | 'previous',
  ): Promise<LobbyStateResponse> {
    return firstValueFrom(
      this.http.post<LobbyStateResponse>(
        `/api/v1/lobbies/${code}/fast-bio/review/advance`,
        { direction },
        { headers: this.authorizationHeaders(reconnectToken) },
      ),
    );
  }

  reactToFastBioProposal(
    code: string,
    reconnectToken: string,
    proposalId: string,
    emoji: string,
  ): Promise<void> {
    return firstValueFrom(
      this.http.post<void>(
        `/api/v1/lobbies/${code}/fast-bio/proposals/${proposalId}/react`,
        { emoji },
        { headers: this.authorizationHeaders(reconnectToken) },
      ),
    );
  }

  replayFastBio(code: string, reconnectToken: string): Promise<LobbyStateResponse> {
    return firstValueFrom(
      this.http.post<LobbyStateResponse>(`/api/v1/lobbies/${code}/fast-bio/replay`, null, {
        headers: this.authorizationHeaders(reconnectToken),
      }),
    );
  }

  private authorizationHeaders(reconnectToken: string): HttpHeaders {
    return new HttpHeaders({ Authorization: `Bearer ${reconnectToken}` });
  }

  private singlePhotoFormData(photo: File): FormData {
    const formData = new FormData();
    formData.append('photo', photo, photo.name);
    return formData;
  }
}
