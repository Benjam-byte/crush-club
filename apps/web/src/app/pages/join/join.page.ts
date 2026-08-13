import { ChangeDetectionStrategy, Component, effect, inject, signal } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { FormControl, FormGroup, ReactiveFormsModule, Validators } from '@angular/forms';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { map } from 'rxjs';
import {
  IonButton,
  IonContent,
  IonIcon,
  IonInput,
  IonSelect,
  IonSelectOption,
} from '@ionic/angular/standalone';
import type { LobbyMode } from '@core/models/game.models';
import { PageHeaderComponent } from '../../core/components/page-header/page-header.component';
import { GameConfigService } from '../../core/services/game-config.service';
import { GameStateService } from '../../core/services/game-state.service';

@Component({
  selector: 'app-join-page',
  imports: [
    IonButton,
    IonContent,
    IonIcon,
    IonInput,
    IonSelect,
    IonSelectOption,
    PageHeaderComponent,
    ReactiveFormsModule,
    RouterLink,
  ],
  templateUrl: './join.page.html',
  styleUrl: './join.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class JoinPage {
  private readonly activatedRoute = inject(ActivatedRoute);
  protected readonly gameConfigs = inject(GameConfigService);
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);

  // Reactive to queryParamMap (not just a one-time snapshot): the Ionic route
  // strategy reuses this page's instance across "/join" <-> "/join?mode=create"
  // navigations (same route config, only the query param differs), so relying
  // on the constructor-time snapshot would leave this stuck on whichever mode
  // was active the first time the page was ever created this session.
  protected readonly isCreateMode = toSignal(
    this.activatedRoute.queryParamMap.pipe(map((params) => params.get('mode') === 'create')),
    { initialValue: this.activatedRoute.snapshot.queryParamMap.get('mode') === 'create' },
  );
  protected readonly hasSubmitted = signal(false);
  protected readonly isSubmitting = signal(false);
  protected readonly selectedMode = signal<LobbyMode>('classic');
  private readonly invitedLobbyCode = (this.activatedRoute.snapshot.queryParamMap.get('code') ?? '')
    .replace(/[^a-zA-Z0-9]/g, '')
    .toUpperCase()
    .slice(0, 6);

  protected readonly joinForm = new FormGroup({
    lobbyCode: new FormControl(this.invitedLobbyCode, {
      nonNullable: true,
      validators: [Validators.required, Validators.minLength(4), Validators.maxLength(6)],
    }),
    displayName: new FormControl('', {
      nonNullable: true,
      validators: [Validators.required, Validators.minLength(2), Validators.maxLength(24)],
    }),
    mode: new FormControl<LobbyMode>('classic', { nonNullable: true }),
    maxPlayers: new FormControl(5, {
      nonNullable: true,
      validators: [Validators.required, Validators.min(3), Validators.max(5)],
    }),
    gameConfigId: new FormControl('', { nonNullable: true }),
  });

  private hasLoadedGameConfigs = false;

  constructor() {
    // An effect (not ngOnInit) so this still fires if the Ionic route
    // strategy reuses this instance and the user only later navigates into
    // create mode (see the isCreateMode comment above).
    effect(() => {
      if (!this.isCreateMode() || this.hasLoadedGameConfigs) {
        return;
      }
      this.hasLoadedGameConfigs = true;
      void this.loadDefaultGameConfig();
    });
  }

  private async loadDefaultGameConfig(): Promise<void> {
    try {
      await this.gameConfigs.initialize();
      const defaultConfig = this.gameConfigs.configList().find((config) => config.kind === 'system')
        ?? this.gameConfigs.configList()[0];
      if (defaultConfig) {
        this.joinForm.controls.gameConfigId.setValue(defaultConfig.id);
      }
    } catch {
      // The service error is displayed in the form.
    }
  }

  protected async onSubmit(): Promise<void> {
    this.hasSubmitted.set(true);
    if (this.joinForm.controls.displayName.invalid) {
      return;
    }
    if (!this.isCreateMode() && this.joinForm.controls.lobbyCode.invalid) {
      return;
    }
    const isFastBioMode = this.selectedMode() === 'fast_bio';
    if (this.isCreateMode() && !isFastBioMode && !this.joinForm.controls.gameConfigId.value) {
      return;
    }

    this.isSubmitting.set(true);
    try {
      const displayName = this.joinForm.controls.displayName.value.trim();
      const lobbyCode = this.isCreateMode()
        ? await this.gameState.createLobby(
            displayName,
            this.selectedMode(),
            this.joinForm.controls.gameConfigId.value,
            isFastBioMode ? undefined : this.joinForm.controls.maxPlayers.value,
          )
        : await this.gameState.joinLobby(displayName, this.joinForm.controls.lobbyCode.value);
      await this.router.navigate(['/lobby', lobbyCode]);
    } catch {
      // GameState exposes the actionable error message.
    } finally {
      this.isSubmitting.set(false);
    }
  }

  protected onLobbyCodeInput(): void {
    const normalizedCode = this.joinForm.controls.lobbyCode.value
      .replace(/[^a-zA-Z0-9]/g, '')
      .toUpperCase()
      .slice(0, 6);
    this.joinForm.controls.lobbyCode.setValue(normalizedCode, {
      emitEvent: false,
    });
  }

  protected onModeChange(event: Event): void {
    const mode = (event as CustomEvent<{ value: LobbyMode }>).detail.value;
    this.selectedMode.set(mode);
    this.joinForm.controls.mode.setValue(mode, { emitEvent: false });
  }
}
