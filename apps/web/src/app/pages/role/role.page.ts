import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import { BrandMarkComponent } from '../../core/components/brand-mark/brand-mark.component';
import { ProfilePortraitComponent } from '../../core/components/profile-portrait/profile-portrait.component';
import { GameStateService } from '../../core/services/game-state.service';

@Component({
  selector: 'app-role-page',
  imports: [BrandMarkComponent, IonButton, IonContent, IonIcon, ProfilePortraitComponent],
  templateUrl: './role.page.html',
  styleUrl: './role.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class RolePage {
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);

  protected onStartQuestionnaire(): void {
    void this.router.navigate(['/game/demo/round/1']);
  }
}
