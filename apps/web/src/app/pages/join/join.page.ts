import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';
import type { OnInit } from '@angular/core';
import { FormControl, FormGroup, ReactiveFormsModule, Validators } from '@angular/forms';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import {
  IonButton,
  IonContent,
  IonIcon,
  IonInput,
  IonSelect,
  IonSelectOption,
} from '@ionic/angular/standalone';
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
export class JoinPage implements OnInit {
  private readonly activatedRoute = inject(ActivatedRoute);
  protected readonly gameConfigs = inject(GameConfigService);
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);

  protected readonly isCreateMode = signal(
    this.activatedRoute.snapshot.queryParamMap.get('mode') === 'create',
  );
  protected readonly hasSubmitted = signal(false);
  protected readonly isSubmitting = signal(false);
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
    gameConfigId: new FormControl('', { nonNullable: true }),
  });

  async ngOnInit(): Promise<void> {
    if (!this.isCreateMode()) {
      return;
    }
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
    if (this.isCreateMode() && !this.joinForm.controls.gameConfigId.value) {
      return;
    }

    this.isSubmitting.set(true);
    try {
      const displayName = this.joinForm.controls.displayName.value.trim();
      const lobbyCode = this.isCreateMode()
        ? await this.gameState.createLobby(
            displayName,
            this.joinForm.controls.gameConfigId.value,
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
}
