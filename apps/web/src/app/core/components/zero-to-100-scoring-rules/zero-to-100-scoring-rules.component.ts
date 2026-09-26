import { ChangeDetectionStrategy, Component } from '@angular/core';
import { IonIcon } from '@ionic/angular/standalone';

/** Static reminder of how 0 à 100 rounds are scored, shown while players wait for the others. */
@Component({
  selector: 'app-zero-to-100-scoring-rules',
  imports: [IonIcon],
  templateUrl: './zero-to-100-scoring-rules.component.html',
  styleUrl: './zero-to-100-scoring-rules.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ZeroToHundredScoringRulesComponent {}
