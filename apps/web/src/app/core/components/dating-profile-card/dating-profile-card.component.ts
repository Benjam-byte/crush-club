import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { IonIcon } from '@ionic/angular/standalone';
import { ProfilePortraitComponent } from '../profile-portrait/profile-portrait.component';

@Component({
  selector: 'app-dating-profile-card',
  imports: [IonIcon, ProfilePortraitComponent],
  templateUrl: './dating-profile-card.component.html',
  styleUrl: './dating-profile-card.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class DatingProfileCardComponent {
  readonly tagline = input.required<string>();
  readonly bio = input.required<string>();
  readonly avatarIndex = input(0);
  readonly isOfficial = input(false);
  readonly authorName = input<string>();
  readonly matchPercentage = input<number>();
  readonly isCompact = input(false);
}
