# Web client render settings design

## Goal

Add a Settings window to `web_new` where a player can choose the actor render
mode: `hybrid3d` or `baked8`.

## UI

The existing Settings action in the left HUD rail opens and closes the standard
draggable game window. The window contains one Renderer section with two
mutually exclusive choices. The selected option is visibly marked and can be
changed with a mouse or keyboard.

## Data flow

A small UI-level settings composable owns the render-mode preference. On
startup it reads the saved value from `localStorage`, validates it against the
two supported modes, and otherwise uses `hybrid3d`. When the user changes the
mode, the composable immediately calls
`gameFacade.setActorRenderSettings({ mode })` and persists the validated value.

The existing `GameFacade -> Render -> ActorRenderer` settings path applies the
change to already loaded actors, so no reconnect or page reload is necessary.

## Error handling and tests

Invalid, malformed, or unavailable saved values fall back safely to
`hybrid3d`. Unit tests cover loading, validation, saving, and the immediate
application callback. The production build provides the integration check.
