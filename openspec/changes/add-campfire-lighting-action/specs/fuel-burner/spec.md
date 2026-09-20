# Spec Delta

## MODIFIED Requirements

### Requirement: Burner duration uses server runtime only
The system SHALL burn one stored fuel unit after each configured duration of accumulated server runtime while the burner is active. A burner with stored initial fuel that has not been activated SHALL preserve that fuel without scheduling or consuming it. Server downtime SHALL NOT advance active burning. The system SHALL preserve enough state to resume the same active or inactive burn state after server restart or world-object unload/reload.

#### Scenario: Campfire initial duration
- **WHEN** a campfire has five initial fuel units, a duration of 1,440 seconds per unit, and is successfully ignited
- **THEN** it SHALL remain fueled for 7,200 seconds of accumulated server runtime from ignition unless it receives more fuel

#### Scenario: Unignited initial fuel is preserved
- **WHEN** a newly constructed campfire has five initial fuel units and remains unlit
- **THEN** accumulated server runtime SHALL not reduce its fuel reserve or create an exhaustion outcome

#### Scenario: Server downtime does not burn fuel
- **WHEN** an active burner is saved and the server is stopped for wall-clock time
- **THEN** its remaining fuel duration SHALL be unchanged when the server resumes
