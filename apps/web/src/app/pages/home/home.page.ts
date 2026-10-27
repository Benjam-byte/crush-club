import { ChangeDetectionStrategy, Component } from '@angular/core';
import { RouterLink } from '@angular/router';
import { IonButton, IonContent, IonIcon, IonModal } from '@ionic/angular/standalone';
import { BrandMarkComponent } from '../../core/components/brand-mark/brand-mark.component';
import { ProfilePortraitComponent } from '../../core/components/profile-portrait/profile-portrait.component';
import { DiagnosticsAccessDirective } from '../../core/directives/diagnostics-access.directive';

@Component({
  selector: 'app-home-page',
  imports: [BrandMarkComponent, DiagnosticsAccessDirective, IonButton, IonContent, IonIcon, IonModal, ProfilePortraitComponent, RouterLink],
  templateUrl: './home.page.html',
  styleUrl: './home.page.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class HomePage {}
