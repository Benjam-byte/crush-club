import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import type { OnInit } from '@angular/core';
import { Router, RouterLink } from '@angular/router';
import {
  IonButton,
  IonContent,
  IonIcon,
} from '@ionic/angular/standalone';
import type { GameConfig } from '@core/models/game.models';
import { PageHeaderComponent } from '../../core/components/page-header/page-header.component';
import { GameConfigService } from '../../core/services/game-config.service';

@Component({
  selector: 'app-game-configs-page',
  imports: [
    IonButton,
    IonContent,
    IonIcon,
    PageHeaderComponent,
    RouterLink,
  ],
  templateUrl: './game-configs.page.html',
  styleUrl: './game-configs.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class GameConfigsPage implements OnInit {
  protected readonly gameConfigs = inject(GameConfigService);
  protected readonly isBusy = signal(false);
  private readonly router = inject(Router);

  protected readonly systemConfigList = computed(() => {
    return this.gameConfigs.configList().filter((config) => config.kind === 'system');
  });
  protected readonly personalConfigList = computed(() => {
    return this.gameConfigs.configList().filter((config) => {
      return config.kind === 'personal' && config.isOwner;
    });
  });
  protected readonly sharedConfigList = computed(() => {
    return this.gameConfigs.configList().filter((config) => {
      return config.kind === 'personal' && config.isPublic && !config.isOwner;
    });
  });

  async ngOnInit(): Promise<void> {
    try {
      await this.gameConfigs.initialize();
    } catch {
      // The service exposes the actionable error message in the template.
    }
  }

  protected async duplicate(config: GameConfig): Promise<void> {
    this.isBusy.set(true);
    try {
      const duplicate = await this.gameConfigs.duplicate(config);
      await this.router.navigate(['/game-configs', duplicate.id, 'edit']);
    } catch {
      // The service exposes the API error.
    } finally {
      this.isBusy.set(false);
    }
  }

  protected async delete(config: GameConfig): Promise<void> {
    if (!window.confirm(`Supprimer « ${config.name} » ? Les lobbys existants conserveront leurs questions.`)) {
      return;
    }
    try {
      await this.gameConfigs.delete(config.id);
    } catch {
      // The service exposes the API error.
    }
  }

  protected async retry(): Promise<void> {
    try {
      await this.gameConfigs.initialize(true);
    } catch {
      // The service exposes the API error.
    }
  }
}
