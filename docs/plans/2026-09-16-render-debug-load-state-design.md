# Render debug load-state design

## Goal

Keep the saved render-debug preference authoritative throughout client startup.

## Design

`GameView` reads the strict boolean preference before creating the renderer and
passes it through `GameFacade`. `Render.init` must not subsequently overwrite
that runtime value with `config.DEBUG`; it will retain the `DebugOverlay`
visibility already selected by the facade. The build-time debug value remains
the fallback only when no saved preference exists.

## Verification

Run the web-client production build to type-check the changed initialization
path.
