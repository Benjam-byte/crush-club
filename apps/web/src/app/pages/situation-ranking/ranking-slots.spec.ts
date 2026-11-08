import { describe, expect, it } from 'vitest';
import {
  clearRankingSlot,
  createRankingSlotList,
  findUnplacedIdList,
  isRankingComplete,
  placeInFirstFreeSlot,
} from './ranking-slots';

const candidateIdList = ['a', 'b', 'c', 'd'];

describe('ranking slots', () => {
  it('starts with every place free', () => {
    expect(createRankingSlotList(candidateIdList)).toEqual([null, null, null, null]);
  });

  it('places a tapped answer in the first free place', () => {
    const slotList = placeInFirstFreeSlot(createRankingSlotList(candidateIdList), 'c', candidateIdList);
    expect(slotList).toEqual(['c', null, null, null]);
    expect(findUnplacedIdList(slotList, candidateIdList)).toEqual(['a', 'b', 'd']);
  });

  it('ignores an answer that is already placed', () => {
    const slotList = placeInFirstFreeSlot(['c', null, null, null], 'c', candidateIdList);
    expect(slotList).toEqual(['c', null, null, null]);
  });

  it('places the last answer on its own', () => {
    let slotList = createRankingSlotList(candidateIdList);
    for (const candidateId of ['d', 'c', 'b']) {
      slotList = placeInFirstFreeSlot(slotList, candidateId, candidateIdList);
    }
    expect(slotList).toEqual(['d', 'c', 'b', 'a']);
    expect(isRankingComplete(slotList)).toBe(true);
  });

  it('fills a lone finalist straight away', () => {
    expect(createRankingSlotList(['a'])).toEqual(['a']);
  });

  it('frees a place without snapping the answer back, and refills it on the next tap', () => {
    const clearedSlotList = clearRankingSlot(['a', 'b', 'c', 'd'], 0);
    expect(clearedSlotList).toEqual([null, 'b', 'c', 'd']);
    expect(isRankingComplete(clearedSlotList)).toBe(false);
    expect(placeInFirstFreeSlot(clearedSlotList, 'a', candidateIdList)).toEqual(['a', 'b', 'c', 'd']);
  });

  it('swaps two answers by freeing both places', () => {
    const clearedSlotList = clearRankingSlot(clearRankingSlot(['a', 'b', 'c', 'd'], 0), 1);
    expect(placeInFirstFreeSlot(clearedSlotList, 'b', candidateIdList)).toEqual(['b', 'a', 'c', 'd']);
  });
});
