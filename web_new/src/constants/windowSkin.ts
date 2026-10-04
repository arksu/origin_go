export const WINDOW_SKIN = {
  frameParts: {
    'top-left': '/assets/img/window-forest/top-left@2x.png',
    top: '/assets/img/window-forest/top@2x.png',
    'top-right': '/assets/img/window-forest/top-right@2x.png',
    left: '/assets/img/window-forest/left@2x.png',
    right: '/assets/img/window-forest/right@2x.png',
    'bottom-left': '/assets/img/window-forest/bottom-left@2x.png',
    bottom: '/assets/img/window-forest/bottom@2x.png',
    'bottom-right': '/assets/img/window-forest/bottom-right@2x.png',
    'left-accent': '/assets/img/window-forest/left-accent@2x.png',
  },
  title: '/assets/img/window-forest/title@2x.png',
  close: '/assets/img/window-forest/close@2x.png',
  density: 2,
  corner: 48,
  // The selected artwork has a longer lower-left branch than its other corners.
  bottomLeftWidth: 72,
  horizontalTileWidth: 32,
  verticalTileHeight: 16,
  leftAccentHeight: 32,
  minimumWidth: 192,
  minimumHeight: 96,
  // Bounds just inside the opaque rails of the selected asymmetric artwork.
  interiorInsets: { left: 20, top: 29, right: 11, bottom: 22 },
  contentPadding: 8,
  panelInset: 16,
  // The asymmetric right rail sits closer to the outside than the left rail.
  panelRight: 8,
  panelTop: 24,
  panelColor: '#293320',
  headerHeight: 32,
  titleHeight: 48,
  titleLineHeight: 24,
  titleTextTop: 10,
  titleTop: 0,
  titleLeft: 0,
  titleLeftCap: 34,
  titleRightCap: 28,
  titleFontSize: 13,
  titleColor: '#e3c178',
  titleEndInset: 24,
  closeSize: 20,
  closeHitSize: 28,
  closeTop: 10,
  closeRight: 0,
  controlGap: 8,
} as const

export function getWindowLayout(innerWidth: number, innerHeight: number) {
  if (!Number.isFinite(innerWidth) || !Number.isFinite(innerHeight) || innerWidth < 0 || innerHeight < 0) {
    throw new Error('GameWindow: content dimensions must be finite non-negative numbers')
  }

  const insets = WINDOW_SKIN.interiorInsets
  // Minimum window dimensions must grow the same padding on all four sides.
  const padding = Math.max(
    WINDOW_SKIN.contentPadding,
    Math.ceil((WINDOW_SKIN.minimumWidth - innerWidth - insets.left - insets.right) / 2),
    Math.ceil((WINDOW_SKIN.minimumHeight - innerHeight - insets.top - insets.bottom) / 2),
  )
  const interiorWidth = innerWidth + padding * 2
  const interiorHeight = innerHeight + padding * 2

  return {
    width: interiorWidth + insets.left + insets.right,
    height: interiorHeight + insets.top + insets.bottom,
    interiorWidth,
    interiorHeight,
    padding,
  }
}
