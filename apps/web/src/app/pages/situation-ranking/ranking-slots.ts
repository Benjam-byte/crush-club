/** One entry per place, best first: the proposal id placed there, or null while the place is free. */
export type RankingSlotList = readonly (string | null)[];

export function createRankingSlotList(candidateIdList: readonly string[]): RankingSlotList {
  return completeLastSlot(candidateIdList.map(() => null), candidateIdList);
}

export function placeInFirstFreeSlot(
  slotList: RankingSlotList,
  candidateId: string,
  candidateIdList: readonly string[],
): RankingSlotList {
  const freeSlotIndex = slotList.indexOf(null);
  if (freeSlotIndex === -1 || slotList.includes(candidateId)) {
    return slotList;
  }
  const nextSlotList = [...slotList];
  nextSlotList[freeSlotIndex] = candidateId;
  return completeLastSlot(nextSlotList, candidateIdList);
}

/** Never auto-completes: the answer just removed would otherwise snap straight back into its place. */
export function clearRankingSlot(slotList: RankingSlotList, slotIndex: number): RankingSlotList {
  const nextSlotList = [...slotList];
  nextSlotList[slotIndex] = null;
  return nextSlotList;
}

export function findUnplacedIdList(slotList: RankingSlotList, candidateIdList: readonly string[]): string[] {
  return candidateIdList.filter((candidateId) => !slotList.includes(candidateId));
}

export function isRankingComplete(slotList: RankingSlotList): slotList is readonly string[] {
  return slotList.every((slot) => slot !== null);
}

/** The last answer can only go in the last free place, so it is placed without waiting for a tap. */
function completeLastSlot(slotList: (string | null)[], candidateIdList: readonly string[]): RankingSlotList {
  const unplacedIdList = findUnplacedIdList(slotList, candidateIdList);
  const freeSlotIndex = slotList.indexOf(null);
  if (unplacedIdList.length !== 1 || freeSlotIndex === -1) {
    return slotList;
  }
  slotList[freeSlotIndex] = unplacedIdList[0];
  return slotList;
}
