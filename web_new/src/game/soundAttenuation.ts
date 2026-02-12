export enum SoundAttenuationModel {
  Linear = 'linear',
  Smoothstep = 'smoothstep',
  Inverse = 'inverse',
}

function normalizedDistance(distance: number, maxHearDistance: number): number {
  if (!Number.isFinite(distance) || !Number.isFinite(maxHearDistance) || maxHearDistance <= 0) {
    return 1
  }
  if (distance <= 0) {
    return 0
  }
  if (distance >= maxHearDistance) {
    return 1
  }
  return distance / maxHearDistance
}

export function linearDistanceAttenuation(distance: number, maxHearDistance: number): number {
  const t = normalizedDistance(distance, maxHearDistance)
  if (t >= 1) {
    return 0
  }
  return 1 - t
}

export function smoothstepDistanceAttenuation(distance: number, maxHearDistance: number): number {
  const t = normalizedDistance(distance, maxHearDistance)
  if (t >= 1) {
    return 0
  }
  const curve = (3 * t * t) - (2 * t * t * t)
  return 1 - curve
}

export function inverseDistanceAttenuation(distance: number, maxHearDistance: number): number {
  const t = normalizedDistance(distance, maxHearDistance)
  if (t >= 1) {
    return 0
  }

  const rolloff = 4
  const maxValue = 1 / (1 + (rolloff * 1))
  const value = 1 / (1 + (rolloff * t))
  return (value - maxValue) / (1 - maxValue)
}

export function distanceAttenuation(
  distance: number,
  maxHearDistance: number,
  model: SoundAttenuationModel,
): number {
  switch (model) {
    case SoundAttenuationModel.Smoothstep:
      return smoothstepDistanceAttenuation(distance, maxHearDistance)
    case SoundAttenuationModel.Inverse:
      return inverseDistanceAttenuation(distance, maxHearDistance)
    case SoundAttenuationModel.Linear:
    default:
      return linearDistanceAttenuation(distance, maxHearDistance)
  }
}
