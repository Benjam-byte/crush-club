/**
 * Points a finalist earns from one vote at this place: first of N earns N, last earns 1.
 * Mirrors the server's Borda count (`tallyProposalPoints` in situation_handlers.go).
 */
export function pointsForPlace(placeIndex: number, finalistCount: number): number {
  return finalistCount - placeIndex;
}

export function placeLabel(place: number): string {
  return place === 1 ? '1re' : `${place}e`;
}

export function pointsLabel(points: number): string {
  return points === 1 ? '1 pt' : `${points} pts`;
}
