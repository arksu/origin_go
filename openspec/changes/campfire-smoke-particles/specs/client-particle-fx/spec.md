# Spec Delta

## Purpose

Let the web client render procedural particle effects attached to world objects, declared per object state in client object definitions, with campfire smoke while burning as the first effect and a config-only path for future effect variants.

## ADDED Requirements

### Requirement: Smoke renders while a campfire is burning
The client SHALL render a rising, drifting particle smoke effect attached to a campfire's view whenever the campfire's appearance resource is the burning state, and SHALL NOT render smoke for any other campfire state. Smoke SHALL rise from the fire area, drift with the ambient wind, and animate continuously while the state holds.

#### Scenario: Burning campfire shows smoke
- **WHEN** the client renders a campfire whose appearance resource is `campfire/burning`
- **THEN** a continuous smoke particle effect SHALL be visible attached to that campfire's view, rendering above its flame layer

#### Scenario: Unlit campfire shows no smoke
- **WHEN** a campfire's appearance resource is not the burning state
- **THEN** no smoke effect SHALL be rendered for that campfire

#### Scenario: Smoke stops when fire goes out
- **WHEN** a burning campfire's appearance changes to the unlit state and its view is rebuilt
- **THEN** the smoke effect SHALL stop for that campfire

### Requirement: Effects attach and detach with the object view lifecycle
The client SHALL attach a declared particle effect when an object view is built with that effect in its state definition, and SHALL remove the effect when that view is destroyed, including on appearance-driven view rebuilds and chunk unloads. An effect removed this way SHALL immediately stop spawning particles; whether its live particles fade out over a short tail or vanish instantly SHALL be configured per effect definition, defaulting to vanish.

#### Scenario: Effect is removed on view destroy
- **WHEN** an object view with an attached effect is destroyed
- **THEN** the effect's emitter SHALL stop spawning and no particle of that effect SHALL outlive the cleanup by more than the configured linger tail

#### Scenario: Default removal is instant
- **WHEN** an effect definition does not declare a linger tail and the effect is removed
- **THEN** its live particles SHALL vanish with the view

### Requirement: Culled objects do not simulate particles
The client SHALL NOT spawn or update particles for an effect whose owner object view is culled from the viewport, and SHALL resume simulation when the view becomes visible again.

#### Scenario: Culled campfire pauses smoke
- **WHEN** a burning campfire's view is culled outside the viewport
- **THEN** its smoke effect SHALL spawn no new particles while culled

#### Scenario: Visible campfire resumes smoke
- **WHEN** a culled burning campfire's view becomes visible again
- **THEN** its smoke effect SHALL resume spawning and updating particles

### Requirement: Wind drifts particles left to right
The client SHALL apply an ambient wind constant to particle motion, flowing horizontally left to right at a fixed average strength. Per-particle wind response SHALL vary within the effect's configured range so individual particles drift at different rates while the wind source itself remains a single static constant.

#### Scenario: Smoke drifts rightward
- **WHEN** smoke particles are simulated for a burning campfire
- **THEN** their horizontal motion SHALL trend to the right, consistent with the wind constant

### Requirement: Effect variants are declared as data
The client SHALL allow an object state definition to declare a particle effect by preset and parameters — including density, rise speed, tint, puff size, sway, and wind response for the smoke preset — such that a new variant of the same effect family renders with the declared characteristics without client code changes. Per-emitter live particle counts SHALL be capped at the configured maximum.

#### Scenario: Denser smoke variant needs no code
- **WHEN** an object state declares the smoke preset with a higher density and a different tint than the campfire's default
- **THEN** the client renders that variant's smoke with the declared density and tint using only the declaration

#### Scenario: Particle count is capped
- **WHEN** an effect's configured maximum live particle count is reached
- **THEN** the emitter SHALL not create additional particles until existing ones expire
