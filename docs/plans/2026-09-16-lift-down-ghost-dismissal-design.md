# Lift Down ghost dismissal

## Goal

End the client-side Lift Down placement flow as soon as the client sends its placement command.

## Design

The canvas left-click handler already sends `sendLiftPutDown` when Lift Down placement mode is active. Immediately after that call, it will invoke the existing `cancelLiftPutDownMode` helper. The helper clears the mode flag and cancels the rendered lift ghost.

This is intentionally independent of the server response. A rejected placement does not re-arm the ghost or placement mode; the player starts a new placement attempt with Lift Down.

## Verification

Run the relevant web client type check or build after the change. If an existing focused test setup covers `GameView`, add a regression test that asserts the ghost cancellation follows command dispatch.
