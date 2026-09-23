const CURSORS: Record<string, string> = {
  lift: "url('/assets/cursor/lift.png') 0 0, pointer",
  lift_down: "url('/assets/cursor/lift_down.png') 0 0, pointer",
}

export function actionCursorCss(cursorId: string): string {
  if (!cursorId) return 'default'
  return CURSORS[cursorId] || 'help'
}
