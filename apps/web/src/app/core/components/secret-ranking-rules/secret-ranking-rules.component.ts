import { ChangeDetectionStrategy, Component } from '@angular/core';
import { IonIcon } from '@ionic/angular/standalone';

/** Static reminder of how a round of « Le classement secret » plays out, shown while players wait. */
@Component({
  selector: 'app-secret-ranking-rules',
  imports: [IonIcon],
  templateUrl: './secret-ranking-rules.component.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class SecretRankingRulesComponent {}
