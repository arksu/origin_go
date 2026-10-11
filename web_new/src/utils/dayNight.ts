/** Shared decorative day/night profile, evaluated only from a server-sampled phase. */
export function getNightOpacity(phase: number | null): number {
  if (phase === null || !Number.isFinite(phase) || phase < 0 || phase >= 1) return 0
  if (phase < 5 / 24 || phase >= 22 / 24) return 1
  if (phase < 6 / 24) return 6 - phase * 24
  return phase <= 21 / 24 ? 0 : phase * 24 - 21
}
