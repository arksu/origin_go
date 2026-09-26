export const SHALLOW_WATER = {
  textureURL: '/assets/game/fx/shallow_water/ripples.png',
  immersionPx: 29,
  transitionMs: 300,
  rippleWidth: 80,
  rippleHeight: 40,
  // The inner front crest meets the body's cut at the ground anchor.
  rippleAnchorY: 0.5,
  idleAlpha: 0.9,
  movingAlpha: 1,
  pulseDistanceTiles: 0.6,
  pulseScale: 0.04,
  settlingMs: 250,
} as const
