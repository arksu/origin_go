import { serverConstants, type ServerConstantsSnapshot } from '../src/network/ServerConstants'

/** Explicit server profiles for tests and standalone renderer previews. */
export const serverProfile: ServerConstantsSnapshot = {
  coordPerTile: 12, chunkSize: 128, tickRate: 10, directionalMovementSupported: true,
  realSecondsPerGameDay: 28800, hoursPerDay: 24, daysPerMonth: 30, monthsPerYear: 12,
}

export function setWorldParams(coordPerTile: number, chunkSize: number, directionalMovementSupported = true, tickRate = 10): void {
  serverConstants.reset()
  const result = serverConstants.accept({ ...serverProfile, coordPerTile, chunkSize, directionalMovementSupported, tickRate })
  if (result !== 'accepted') throw new Error('Invalid test server profile')
}
