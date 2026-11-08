import { ChangeDetectionStrategy, Component, input, signal } from '@angular/core';
import { IonButton, IonContent, IonModal } from '@ionic/angular/standalone';

@Component({
  selector: 'app-info-sheet',
  imports: [IonButton, IonContent, IonModal],
  templateUrl: './info-sheet.component.html',
  styleUrl: './info-sheet.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class InfoSheetComponent {
  readonly label = input('Voir les règles');
  protected readonly isOpen = signal(false);
}
