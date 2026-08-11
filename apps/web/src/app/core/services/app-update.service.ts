import { ApplicationRef, Injectable, inject, signal } from '@angular/core';
import { SwUpdate } from '@angular/service-worker';
import { first } from 'rxjs';

/**
 * Detects when a new build has been deployed and lets the player pick it up
 * without having to know to close every tab. The Angular service worker keeps
 * serving the version it installed on first load until something explicitly
 * asks it to check again, so this checks once the app has finished its
 * initial render.
 */
@Injectable({ providedIn: 'root' })
export class AppUpdateService {
  private readonly swUpdate = inject(SwUpdate);
  private readonly appRef = inject(ApplicationRef);

  readonly updateAvailable = signal(false);

  constructor() {
    if (!this.swUpdate.isEnabled) {
      return;
    }

    this.swUpdate.versionUpdates.subscribe((event) => {
      if (event.type === 'VERSION_READY') {
        this.updateAvailable.set(true);
      } else if (event.type === 'VERSION_INSTALLATION_FAILED') {
        console.error(`App version '${event.version.hash}' failed to install: ${event.error}`);
      }
    });

    // The previous version's assets are gone from the server after a deploy;
    // if the worker ends up in a broken state, a clean reload is the only fix.
    this.swUpdate.unrecoverable.subscribe((event) => {
      console.error(`App state is unrecoverable, reloading: ${event.reason}`);
      this.reload();
    });

    this.appRef.isStable
      .pipe(first((isStable) => isStable))
      .subscribe(() => void this.swUpdate.checkForUpdate());
  }

  /** Reloads to pick up the version the service worker already downloaded. */
  reload(): void {
    document.location.reload();
  }

  dismissUpdateToast(): void {
    this.updateAvailable.set(false);
  }
}
