# player-name-labels Specification

## Purpose

Carry each named entity's display name and role color on the object spawn stream and render persistent nickname labels over them in the world, so players can identify every player character they see, including their own.

## Requirements

### Requirement: Object spawns carry display name and role color
The server SHALL include the entity's display name and name color in every object spawn message for entities that have a display name, and SHALL omit the name for entities without one. The name color SHALL be a semantic role value (default, administrator, NPC, ...); entities with no assigned role SHALL use the default role. A spawn rebuilt after an appearance or equipment change SHALL carry the entity's current name and role color.

#### Scenario: Named player enters view
- **WHEN** a player character spawns into an observer's view
- **THEN** the spawn message SHALL contain that character's nickname and its role color

#### Scenario: Unnamed entity enters view
- **WHEN** an entity without a display name (object, animal, dropped item) spawns into view
- **THEN** the spawn message SHALL carry no name, and the observer SHALL show no label for it

#### Scenario: Respawn carries current name
- **WHEN** a named entity's spawn is rebuilt for an observer after an appearance change
- **THEN** the spawn message SHALL carry the same nickname and role color as the original spawn

### Requirement: Every named entity shows a persistent nickname label
The client SHALL display a nickname label above the head of every entity whose spawn carries a name, including the local player's own entity. The label SHALL persist while the entity exists — it SHALL NOT expire or require chat activity — SHALL track the entity's position and keep a constant on-screen size across zoom levels, and SHALL be hidden whenever the entity's visual is hidden by culling. The label text style SHALL be plain outlined text, colored by the client-side palette entry bound to the spawn's role color; an unrecognized role color SHALL fall back to the default palette entry.

#### Scenario: Remote player becomes visible
- **WHEN** another player's spawn with a name is received
- **THEN** a persistent label with that nickname appears above the player's head

#### Scenario: Local player spawns
- **WHEN** the local player's own entity spawn with a name is received
- **THEN** the local player's nickname label is shown above their head like any other player's

#### Scenario: Zooming
- **WHEN** the camera zooms in or out while a label is visible
- **THEN** the label keeps the same on-screen size and stays anchored above its entity

#### Scenario: Culled entity
- **WHEN** an entity's visual is hidden by the client's culling system
- **THEN** its nickname label is hidden too and never floats detached

### Requirement: Late name and color updates are applied on respawn
When a spawn arrives for an entity the client already renders with an unchanged visual state, the client SHALL still apply the spawn's name and role color to the existing entity's label instead of ignoring them.

#### Scenario: Respawn with changed role color
- **WHEN** a respawn for a visible entity reports the same visual generation but a different role color
- **THEN** the entity's existing label is recolored without duplicating the entity or the label

### Requirement: Labels clean up with the entity and the world
The client SHALL remove an entity's nickname label when the entity despawns, and SHALL clear all labels when the world is reset or the stream epoch changes.

#### Scenario: Entity despawns
- **WHEN** a named entity despawns from view
- **THEN** its nickname label is removed in the same update

#### Scenario: World reset
- **WHEN** the client resets the world (reconnect or stream epoch change)
- **THEN** all nickname labels are cleared and rebuilt only from new spawns

### Requirement: Chat balloons stack above the nickname
When a chat balloon and a nickname label are visible for the same entity at the same time, the balloon SHALL be positioned one label-height above the nickname so the two never overlap; the nickname keeps its anchor at the entity's visual top, lowered slightly into the visual by a client-side style constant.

#### Scenario: Player chats while labeled
- **WHEN** a chat balloon is shown for an entity whose nickname label is visible
- **THEN** the balloon floats directly above the nickname without overlapping it
