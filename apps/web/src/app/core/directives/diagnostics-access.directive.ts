import { Directive, inject } from '@angular/core';
import { Router } from '@angular/router';

@Directive({
  selector: '[appDiagnosticsAccess]',
  host: {
    '(click)': 'onTap()',
  },
})
export class DiagnosticsAccessDirective {
  private readonly router = inject(Router);
  private tapCount = 0;
  private firstTapAt = 0;

  onTap(): void {
    const now = Date.now();
    if (this.tapCount === 0 || now - this.firstTapAt > 2000) {
      this.tapCount = 0;
      this.firstTapAt = now;
    }
    this.tapCount++;
    if (this.tapCount < 4) {
      return;
    }
    this.tapCount = 0;
    void this.router.navigate(['/diagnostics']);
  }
}
