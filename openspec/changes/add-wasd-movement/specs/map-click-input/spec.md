# Spec Delta

## ADDED Requirements

### Requirement: Pointer gameplay input explicitly releases keyboard control
When keyboard movement is active, a primary or secondary map click, touch long-press, or placement/hand-drop click that will issue gameplay input SHALL release keyboard control before issuing its normal command. The release SHALL be a directional stop, not a gameplay-action cancellation. The client SHALL then retain existing click routing and send exactly one normal map click or existing placement/inventory operation. Without keyboard control it SHALL send no additional movement stop. Camera pan, zoom, pointer motion, and non-gameplay UI interaction SHALL NOT count as a map-command handoff.

The keys held at handoff SHALL be suppressed until released; their repeat or release SHALL NOT take control back or stop a newly installed route. A fresh key press SHALL be able to acquire directional control again. This rule SHALL NOT change the existing server interpretation of a standalone MapClick, including a secondary ground click leaving unrelated point/entity movement unchanged.

#### Scenario: Click while holding W
- **WHEN** the player holds W and primary-clicks a distant point
- **THEN** the client SHALL send directional release before the ordinary MapClick and the server SHALL follow the clicked route

#### Scenario: Old keyup cannot cancel the route
- **WHEN** the player releases W after that handoff
- **THEN** no further directional stop SHALL be sent for the old hold and the click route SHALL continue

#### Scenario: Secondary ground during keyboard movement
- **WHEN** the player secondary-clicks empty ground while WASD is active
- **THEN** the preceding directional stop SHALL end keyboard movement, and the secondary MapClick SHALL retain its existing action-cancellation and ground-routing behavior

#### Scenario: Mouse-only secondary ground
- **WHEN** keyboard control is inactive and the player secondary-clicks ground during an ordinary click route
- **THEN** exactly one secondary MapClick SHALL be sent, with no directional stop, and unrelated route movement SHALL remain unchanged

#### Scenario: Placement and inventory hand
- **WHEN** a click is handled by an existing placement callback or hand-item drop while keyboard movement is active
- **THEN** keyboard control SHALL be released before that operation, without also emitting an ordinary MapClick or a duplicate operation

#### Scenario: Camera input
- **WHEN** the player pans or zooms the camera while holding a movement direction
- **THEN** directional control SHALL continue unchanged

### Requirement: Directional input never consumes map selections
Directional input SHALL not be interpreted as a map click and SHALL NOT select or consume pending administrator commands, target a gameplay action, drop a held item, or initiate carry placement. Fresh WASD movement SHALL supersede an ordinary click route and its pending pickup or link intent without executing the superseded interaction.

#### Scenario: Pending administrator command
- **WHEN** the player uses WASD while a click-target administrator command is pending
- **THEN** the command SHALL remain pending for an actual primary MapClick

#### Scenario: Pending pickup or link
- **WHEN** WASD replaces an approach to a dropped item or object
- **THEN** the old pickup/link intent SHALL be removed before manual movement and SHALL not complete later

#### Scenario: Held item and carried object
- **WHEN** a player uses WASD with an inventory item in hand or a carried world object
- **THEN** the item/object SHALL remain held or carried, subject to existing movement restrictions
