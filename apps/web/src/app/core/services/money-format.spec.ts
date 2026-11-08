import { describe, expect, it } from 'vitest';
import { formatEuros, formatEurosCompact, maximumSecretRankingAmount, parseTypedAmount } from './money-format';

/** Intl separates thousands with narrow no-break spaces: compare on plain spaces. */
function plain(text: string): string {
  return text.replace(/[\u00a0\u202f]/g, ' ');
}

describe('formatEuros', () => {
  it('groups the thousands of whole euros', () => {
    expect(plain(formatEuros(12500))).toBe('12 500 €');
    expect(plain(formatEuros(0))).toBe('0 €');
  });
});

describe('formatEurosCompact', () => {
  it('gives the order of magnitude of long amounts', () => {
    expect(plain(formatEurosCompact(12_500))).toBe('12,5 k€');
    expect(plain(formatEurosCompact(3_000_000))).toBe('3 M€');
    expect(plain(formatEurosCompact(1_000_000_000))).toBe('1 Md€');
  });

  it('never rounds an amount up to the next unit', () => {
    expect(plain(formatEurosCompact(999_999))).toBe('999,9 k€');
  });

  it('keeps small amounts as they are', () => {
    expect(plain(formatEurosCompact(950))).toBe('950 €');
  });
});

describe('parseTypedAmount', () => {
  it('keeps only the digits', () => {
    expect(parseTypedAmount('12 500 €')).toBe(12500);
    expect(parseTypedAmount('007')).toBe(7);
    expect(parseTypedAmount('0')).toBe(0);
  });

  it('is empty without digits', () => {
    expect(parseTypedAmount('')).toBeNull();
    expect(parseTypedAmount('€')).toBeNull();
  });

  it('caps the amount to the largest bet', () => {
    expect(parseTypedAmount('99999999999999')).toBe(maximumSecretRankingAmount);
  });
});
