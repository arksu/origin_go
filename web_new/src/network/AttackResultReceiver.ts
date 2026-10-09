import type { proto } from './proto/packets'
import { compareUint64, decodeNonzeroUint64 } from '../types/networkIdentity'

const MAX_ATTACK_HITS = 512

/** Accepts ordered combat notifications without predicting health or creating entities. */
export class AttackResultReceiver {
  private epoch = 0
  private lastEventId = '0'

  reset(epoch = 0): void {
    this.epoch = Number.isInteger(epoch) && epoch > 0 && epoch <= 4294967295 ? epoch : 0
    this.lastEventId = '0'
  }

  accept(message: proto.IS2C_AttackResult): boolean {
    if (!this.epoch || message.streamEpoch !== this.epoch) return false
    const eventId = decodeNonzeroUint64(message.eventId), attackerId = decodeNonzeroUint64(message.attackerId)
    if (eventId == null || attackerId == null || compareUint64(eventId, this.lastEventId) <= 0) return false
    const hits = message.hits ?? []
    if (!Array.isArray(hits) || hits.length > MAX_ATTACK_HITS) return false
    const targets = new Set<string>()
    for (const hit of hits) {
      if (hit == null || typeof hit !== 'object') return false
      const targetId = decodeNonzeroUint64(hit.targetId), damage = hit.damage ?? 0
      if (targetId == null || targetId === attackerId || targets.has(targetId) ||
          typeof damage !== 'number' || !Number.isFinite(damage) || damage < 0) return false
      targets.add(targetId)
    }
    // Invalid packets never consume an event ID. Accepted packets retain no payload.
    this.lastEventId = eventId
    return true
  }
}
