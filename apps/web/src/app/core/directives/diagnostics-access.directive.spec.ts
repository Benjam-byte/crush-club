import '@angular/compiler';
import { Injector, runInInjectionContext } from '@angular/core';
import { Router } from '@angular/router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { DiagnosticsAccessDirective } from './diagnostics-access.directive';

describe('DiagnosticsAccessDirective', () => {
  const navigate = vi.fn().mockResolvedValue(true);
  let directive: DiagnosticsAccessDirective;

  beforeEach(() => {
    vi.useFakeTimers();
    navigate.mockClear();
    const injector = Injector.create({ providers: [{ provide: Router, useValue: { navigate } }] });
    directive = runInInjectionContext(injector, () => new DiagnosticsAccessDirective());
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('opens diagnostics only on the fourth consecutive tap', () => {
    for (let tap = 0; tap < 3; tap++) {
      directive.onTap();
      vi.advanceTimersByTime(200);
      expect(navigate).not.toHaveBeenCalled();
    }
    directive.onTap();
    expect(navigate).toHaveBeenCalledExactlyOnceWith(['/diagnostics']);
    directive.onTap();
    expect(navigate).toHaveBeenCalledTimes(1);
  });

  it('does not combine taps made more than two seconds apart', () => {
    directive.onTap();
    directive.onTap();
    vi.advanceTimersByTime(2100);
    directive.onTap();
    directive.onTap();
    expect(navigate).not.toHaveBeenCalled();
    directive.onTap();
    directive.onTap();
    expect(navigate).toHaveBeenCalledExactlyOnceWith(['/diagnostics']);
  });
});
