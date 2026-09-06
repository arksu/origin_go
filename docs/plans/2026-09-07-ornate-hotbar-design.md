# Ornate RPG hotbar

The player selected the ornate RPG direction: textured frames and painted icons.

## Design

Keep the top-centered ten-slot hotbar and left action rail. Give them a shared aged-bronze and dark-leather treatment, engraved corner details, painted fantasy icons, legible ivory labels, and contrasting keycaps numbered 1–9 and 0. Ornament must not obscure the icon silhouettes or interaction states. Use larger touch targets and a responsive layout.

Create one painted icon atlas for the seven existing actions: cog, hand, hammer and anvil, timber frame, character portrait, backpack, and leather tunic. Keep the generated asset and generation prompt in the repository. Share icon rendering across the hotbar, action rail, and slot chooser.

Empty slots open a labeled action chooser. Occupied slots activate their existing action, with right-click or an explicit edit control for changing/removing assignments. Keep rail drag-and-drop, character-specific saved assignments, and number shortcuts. Provide keyboard navigation, focus restoration, touch-safe cancellation, and visible drag targets. Mark the existing unimplemented Settings and Actions entries as unavailable rather than offering silent no-op controls. Open-window highlights reflect the existing GameView computed state.

## Implementation plan

1. Generate and inspect the painted atlas; add shared icon and decorative frame styling.
2. Refactor the hotbar into one slot loop with shortcut badges, action labels, accessible assignment editing, and drop feedback.
3. Restyle the action rail, reuse the icons, and avoid duplicate touch activation.
4. Connect active-window states in GameView; preserve existing network and persistence boundaries.
5. Run the production build and inspect desktop and narrow-screen rendering in a local component fixture. Exercise assignment, removal, keyboard focus, drag-and-drop, and touch cancellation.

No server or gameplay changes are needed. Existing unrelated working-tree changes remain outside this work.
