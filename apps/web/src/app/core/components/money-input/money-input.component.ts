import { ChangeDetectionStrategy, Component, computed, input, model, signal } from '@angular/core';
import { formatEurosCompact, maximumSecretRankingAmount, parseTypedAmount } from '@core/services/money-format';

const groupedNumberFormatter = new Intl.NumberFormat('fr-FR', { maximumFractionDigits: 0 });

/**
 * Large whole-euro amount field: the digits are grouped as they are typed on the numeric keyboard, and
 * the k / M chips multiply the amount for the big numbers.
 */
@Component({
  selector: 'app-money-input',
  templateUrl: './money-input.component.html',
  styleUrl: './money-input.component.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class MoneyInputComponent {
  readonly value = model<number | null>(null);
  readonly disabled = input(false);
  readonly label = input('Ta mise secrète');

  /** Alternates on every change so the bump animation replays. */
  protected readonly bumpParity = signal(false);
  protected readonly displayValue = computed(() => {
    const value = this.value();
    return value === null ? '' : groupedNumberFormatter.format(value);
  });
  protected readonly compactLabel = computed(() => {
    const value = this.value();
    return value !== null && value >= 1000 ? `≈ ${formatEurosCompact(value)}` : null;
  });

  protected onInput(event: Event): void {
    const field = event.target as HTMLInputElement;
    const amount = parseTypedAmount(field.value);
    this.setValue(amount);
    // Rewrite the field right away: the grouping and the cap must show while typing.
    field.value = this.displayValue();
  }

  protected multiply(factor: number): void {
    const current = this.value();
    this.setValue(Math.min((current === null || current === 0 ? 1 : current) * factor, maximumSecretRankingAmount));
  }

  protected clear(): void {
    this.setValue(null);
  }

  private setValue(amount: number | null): void {
    if (amount === this.value()) {
      return;
    }
    this.value.set(amount);
    this.bumpParity.update((parity) => !parity);
  }
}
