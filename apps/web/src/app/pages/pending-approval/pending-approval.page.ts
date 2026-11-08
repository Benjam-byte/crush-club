import { ChangeDetectionStrategy, Component, computed, inject } from '@angular/core';
import { Router } from '@angular/router';
import { IonButton, IonContent, IonIcon } from '@ionic/angular/standalone';
import { PageHeaderComponent } from '@core/components/page-header/page-header.component';
import { WaitingCardComponent } from '@core/components/waiting-card/waiting-card.component';
import { GameStateService } from '@core/services/game-state.service';

/** A player who joined a game in progress waits here until the host lets them in or turns them down. */
@Component({
  selector: 'app-pending-approval-page',
  imports: [IonButton, IonContent, IonIcon, PageHeaderComponent, WaitingCardComponent],
  templateUrl: './pending-approval.page.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class PendingApprovalPage {
  protected readonly gameState = inject(GameStateService);
  private readonly router = inject(Router);

  protected readonly waitingTitle = computed(() => {
    const hostName = this.gameState.hostPlayer()?.displayName;
    return hostName ? `${hostName} doit accepter ta demande` : 'L’hôte doit accepter ta demande';
  });

  /** Going home hands over to the lobby exit guard, which confirms and withdraws the request. */
  protected onCancel(): void {
    void this.router.navigateByUrl('/');
  }
}
