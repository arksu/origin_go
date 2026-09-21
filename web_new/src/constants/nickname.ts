import { proto } from '@/network/proto/packets.js'

// Nickname labels over named world entities (see NicknameManager)
export const NICKNAME_FONT_SIZE = 12
export const NICKNAME_OUTLINE_WIDTH = 3
export const NICKNAME_OUTLINE_COLOR = 0x000000
// Below chat balloons (1_000_000, see ChatBalloonManager), above objects and FX.
export const NICKNAME_Z_INDEX = 900_000
// Approximate rendered label height; the chat balloon floats this far above the
// nickname. Keep in sync with NICKNAME_FONT_SIZE.
export const NICKNAME_LABEL_HEIGHT = 18
// Drops the label (and the balloon above it) 15px lower into the entity's top
// edge so both sit closer to the character's head.
export const NICKNAME_Y_OFFSET_PX = 27

// The server sends a semantic role (NicknameColor); the client owns the look.
// Unknown roles fall back to the default color.
export const NICKNAME_DEFAULT_COLOR = 0xffffff
export const NICKNAME_PALETTE: Record<number, number> = {
  [proto.NicknameColor.NICKNAME_COLOR_DEFAULT]: NICKNAME_DEFAULT_COLOR,
}
