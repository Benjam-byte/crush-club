/** Largest bet of « Le classement secret », mirrored from the API. */
export const maximumSecretRankingAmount = 1_000_000_000;

const euroFormatter = new Intl.NumberFormat('fr-FR', {
  style: 'currency',
  currency: 'EUR',
  maximumFractionDigits: 0,
});

const compactNumberFormatter = new Intl.NumberFormat('fr-FR', { maximumFractionDigits: 1 });

/** "12 500 €": whole euros, French grouping. */
export function formatEuros(amount: number): string {
  return euroFormatter.format(amount);
}

/** "12,5 k€", "3 M€", "1 Md€": a readable order of magnitude under a long amount. */
export function formatEurosCompact(amount: number): string {
  const steps: readonly [number, string][] = [
    [1_000_000_000, 'Md€'],
    [1_000_000, 'M€'],
    [1_000, 'k€'],
  ];
  for (const [step, unit] of steps) {
    if (amount >= step) {
      return `${compactNumberFormatter.format(Math.floor((amount / step) * 10) / 10)} ${unit}`;
    }
  }
  return formatEuros(amount);
}

/** Keeps the digits of a typed amount, capped to the largest bet; null when nothing is left. */
export function parseTypedAmount(typed: string): number | null {
  const digits = typed.replace(/\D/g, '').replace(/^0+(?=\d)/, '');
  if (digits === '') {
    return null;
  }
  return Math.min(Number(digits.slice(0, 10)), maximumSecretRankingAmount);
}
