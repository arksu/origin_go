# Spec Delta

## MODIFIED Requirements

### Requirement: One-shot object selection reports authoritative runtime state
While inspection is armed on the server, the next ordinary primary map click on a currently visible object MUST select that object's entity through `MapClick.target_entity_id` rather than performing normal movement, pickup, or object interaction. The client MUST send ordinary map-click input without recognizing administrator command text or entering a selection mode. The server MUST resolve the entity against the authoritative live world before reporting its state. Explicit `Interact` actions MUST NOT consume pending inspection.

#### Scenario: Burner fuel is reported
- **WHEN** an administrator arms inspection and selects a live campfire with burner fuel remaining
- **THEN** the chat report SHALL include the campfire's current `burner` behavior state and its `fuel` value

#### Scenario: Target disappears before inspection
- **WHEN** an administrator selects an object that no longer exists in the authoritative world
- **THEN** the system SHALL report that the target is unavailable and SHALL NOT inspect another object

#### Scenario: Client has no administrator selection mode
- **WHEN** an administrator sends `/info` and then primary-clicks an object
- **THEN** the client SHALL send its ordinary map-click action and only the server SHALL interpret that click as inspection

#### Scenario: Explicit interaction leaves inspection pending
- **WHEN** an administrator awaiting inspection sends `Interact` through normal context interaction
- **THEN** ordinary interaction SHALL proceed and inspection SHALL remain pending for a map click
