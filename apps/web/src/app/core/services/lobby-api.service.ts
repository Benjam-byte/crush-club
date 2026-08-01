import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import type { LobbyResponse, QuestionnaireSnapshot } from '@core/models/game.models';

export interface CreateLobbyInput {
  displayName: string
  adultConfirmed: boolean
  maxPlayers: number
  gameConfigId: string
}

@Injectable({
  providedIn: 'root',
})
export class LobbyApiService {
  private readonly http = inject(HttpClient);

  create(input: CreateLobbyInput): Promise<LobbyResponse> {
    return firstValueFrom(this.http.post<LobbyResponse>('/api/v1/lobbies', input));
  }

  get(code: string): Promise<LobbyResponse> {
    return firstValueFrom(this.http.get<LobbyResponse>(`/api/v1/lobbies/${code}`));
  }

  changeConfig(code: string, gameConfigId: string): Promise<LobbyResponse> {
    return firstValueFrom(
      this.http.put<LobbyResponse>(`/api/v1/lobbies/${code}/game-config`, { gameConfigId }),
    );
  }

  getQuestionnaire(code: string): Promise<QuestionnaireSnapshot> {
    return firstValueFrom(
      this.http.get<QuestionnaireSnapshot>(`/api/v1/lobbies/${code}/questionnaire`),
    );
  }

  start(code: string): Promise<LobbyResponse> {
    return firstValueFrom(
      this.http.post<LobbyResponse>(`/api/v1/lobbies/${code}/start`, null),
    );
  }
}
