import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

@Component({
  selector: 'app-profile-portrait',
  templateUrl: './profile-portrait.component.html',
  styleUrl: './profile-portrait.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class ProfilePortraitComponent {
  readonly avatarIndex = input(0);
  readonly atlas = input<'players' | 'camille'>('players');
  readonly alt = input.required<string>();

  protected readonly portraitClass = computed(() => {
    const normalizedIndex = Math.max(0, Math.min(3, this.avatarIndex()));
    return `profile-portrait--${this.atlas()} profile-portrait--position-${normalizedIndex}`;
  });
}
