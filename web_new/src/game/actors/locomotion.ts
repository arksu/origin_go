import { proto } from '@/network/proto/packets.js'

export const GAIT_CLIPS = ['crawl', 'walk', 'run', 'fast_run'] as const
export type GaitClip = typeof GAIT_CLIPS[number]
export type LocomotionClip = GaitClip | 'carry_walk'

export function locomotionClip(mode: number, carrying: boolean): LocomotionClip {
  if (carrying) return 'carry_walk'
  switch (mode) {
    case proto.MovementMode.MOVE_MODE_CRAWL: return 'crawl'
    case proto.MovementMode.MOVE_MODE_RUN: return 'run'
    case proto.MovementMode.MOVE_MODE_FAST_RUN: return 'fast_run'
    default: return 'walk'
  }
}
