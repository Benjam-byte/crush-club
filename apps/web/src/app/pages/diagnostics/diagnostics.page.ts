import { DatePipe, DecimalPipe } from '@angular/common';
import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import type { OnDestroy } from '@angular/core';
import { RouterLink } from '@angular/router';
import { IonButton, IonContent, IonToggle } from '@ionic/angular/standalone';
import { PageHeaderComponent } from '../../core/components/page-header/page-header.component';
import { GameStateService } from '../../core/services/game-state.service';
import { LobbyDiagnosticsService } from '../../core/services/lobby-diagnostics.service';
import { RealtimeLobbyService } from '../../core/services/realtime-lobby.service';
import type { LobbyApiProbeResult, WebSocketProbeResult } from '../../core/models/realtime-diagnostics.models';

@Component({
  selector: 'app-diagnostics-page',
  imports: [DatePipe, DecimalPipe, IonButton, IonContent, IonToggle, PageHeaderComponent, RouterLink],
  templateUrl: './diagnostics.page.html',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class DiagnosticsPage implements OnDestroy {
  protected readonly realtime = inject(RealtimeLobbyService);
  protected readonly diagnostics = inject(LobbyDiagnosticsService);
  protected readonly gameState = inject(GameStateService);
  protected readonly lobbyCode = this.gameState.lobbyCode;
  protected readonly feedback = signal('');
  protected readonly reportText = signal('');
  protected readonly history = computed(() => [...this.realtime.diagnosticEvents()].reverse());
  protected readonly checks = computed(() => [...this.diagnostics.checks()].reverse());
  protected readonly recentDisconnectCount = computed(() => this.realtime.diagnosticEvents()
    .filter((event) => event.kind === 'timeout' || (event.kind === 'closed' && !event.intentional)).length);
  protected readonly connectionLabel = computed(() => {
    switch (this.realtime.connectionStatus()) {
      case 'connected': return 'Connecté';
      case 'connecting': return 'Connexion en cours';
      case 'offline': return 'Reconnexion en cours';
      case 'idle': return 'Aucun lobby connecté';
      case 'degraded': return 'Connexion ouverte, état indisponible';
      case 'rejected': return 'Session refusée';
    }
  });
  protected readonly compressionLabel = computed(() => {
    const active = this.realtime.compressionActive();
    return active === null ? 'En attente de connexion' : active ? 'Active' : 'Inactive';
  });

  ngOnDestroy(): void {
    this.diagnostics.stop();
  }

  ionViewWillLeave(): void {
    this.diagnostics.stop();
  }

  protected onCompressionChange(event: CustomEvent<{ checked: boolean }>): void {
    this.realtime.setCompressionEnabled(event.detail.checked);
  }

  protected apiResultLabel(result: LobbyApiProbeResult): string {
    if (result.status === 'ok') return 'HTTP 200 · ' + result.durationMs + ' ms';
    if (result.httpStatus !== null) return 'HTTP ' + result.httpStatus;
    return result.status === 'timeout' ? 'Délai dépassé' : 'Échec réseau';
  }

  protected websocketResultLabel(result: WebSocketProbeResult): string {
    switch (result.status) {
      case 'ok': return 'Réponse reçue · ' + result.durationMs + ' ms';
      case 'timeout': return 'Aucune réponse sous 5 s';
      case 'closed': return 'Connexion interrompue';
      case 'not_connected': return 'WebSocket non connecté';
    }
  }

  protected async onCopyReport(): Promise<void> {
    const report = this.diagnostics.buildReport();
    try {
      if (!navigator.clipboard?.writeText) throw new Error('Clipboard unavailable');
      await navigator.clipboard.writeText(report);
      this.reportText.set('');
      this.feedback.set('Rapport copié.');
    } catch {
      this.reportText.set(report);
      this.feedback.set('Copie automatique indisponible. Sélectionne le rapport ci-dessous ou télécharge-le.');
    }
  }

  protected onDownloadReport(): void {
    const report = new Blob([this.diagnostics.buildReport()], { type: 'application/json' });
    const url = URL.createObjectURL(report);
    const link = document.createElement('a');
    link.href = url;
    link.download = 'crush-club-diagnostic-' + new Date().toISOString().replaceAll(':', '-') + '.json';
    document.body.appendChild(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    this.feedback.set('Rapport téléchargé.');
  }
}
