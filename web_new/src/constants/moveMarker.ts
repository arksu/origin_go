// Ground marker shown where the server targets the local player's move
// (see MoveMarkerManager). Path is resolved by ResourceLoader relative to
// /assets/game/.
export const MOVE_MARKER_TEXTURE = 'move_marker.png'

// The marker pulses smoothly: transparency eases from opaque to transparent
// and back, taking this interval per direction.
export const MOVE_MARKER_BLINK_INTERVAL_MS = 700

// Alpha range of the pulse: the marker never becomes fully invisible and
// never reaches full opacity.
export const MOVE_MARKER_MIN_ALPHA = 0.3
export const MOVE_MARKER_MAX_ALPHA = 0.9
