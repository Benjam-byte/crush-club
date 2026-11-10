import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { IonIcon } from '@ionic/angular/standalone';
import { awardNames } from '../../services/secret-ranking';
import type { SecretRankingAward } from '../../services/secret-ranking';

/** Secret ranking awards, one row each; a tap reveals the detail and the full list of names. */
@Component({
  selector: 'app-award-list',
  imports: [IonIcon],
  templateUrl: './award-list.component.html',
  styleUrl: './award-list.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class AwardListComponent {
  readonly awards = input.required<readonly SecretRankingAward[]>();
  protected readonly awardNames = awardNames;
}
