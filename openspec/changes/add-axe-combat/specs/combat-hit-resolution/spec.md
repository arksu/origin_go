# Spec Delta

## Purpose

Resolve non-target attacks against authoritative world geometry and apply deterministic fractional damage through a shared receiver path, initially verified with isolated server-owned test targets.

## ADDED Requirements

### Requirement: Axe contact uses the collider at impact
At the end of windup, the server SHALL perform one sector/collider intersection check using current collision-resolved positions. The sector SHALL originate at the attacker's current position with direction locked at start. Any collider contact with the 90-degree sector within range 18 SHALL count, including boundary contact. A collider's center need not be inside. Occlusion SHALL NOT be checked.

#### Scenario: Target leaves during windup
- **WHEN** the target's entire collider leaves the sector before impact
- **THEN** it SHALL receive no hit while payment, recovery, and cooldown remain in force

#### Scenario: Large target center outside
- **WHEN** a target center lies outside the angle or range but part of its collider intersects the sector
- **THEN** it SHALL be included in the contact set

#### Scenario: Boundary contact
- **WHEN** a collider touches an angular edge or the outer range boundary
- **THEN** contact SHALL count; a collider separated beyond the documented numerical tolerance SHALL not count

#### Scenario: Attacker moves and changes aim
- **WHEN** the attacker moves during windup and the pointer turns elsewhere
- **THEN** impact SHALL use the new attacker position and the original locked direction

#### Scenario: Wall between attacker and target
- **WHEN** a wall separates the attacker from an intersecting test target
- **THEN** both axe actions SHALL consider that target without a line-of-sight rejection

### Requirement: Candidate eligibility is shared and excludes self
Both axe actions SHALL exclude the attacker's stable ID before geometric testing or ranking. Other candidates SHALL exist in the current world, be undestroyed, have positive health through an enabled damage receiver, and have contact geometry. Selection SHALL NOT filter by friendship or prioritize creature categories. The phase-1 range SHALL enable only test receivers; this isolation SHALL NOT become a permanent world-object type whitelist.

#### Scenario: Only self intersects
- **WHEN** the attacker is the only entity intersecting its sector
- **THEN** both actions SHALL miss without self-damage or refund

#### Scenario: Receiver is gone or depleted
- **WHEN** a candidate is removed, belongs to a stale incarnation, or has zero HP at resolution
- **THEN** it SHALL NOT receive another hit or block selection of an eligible candidate

#### Scenario: Social label does not protect a receiver
- **WHEN** an eligible test receiver is marked as allied
- **THEN** it SHALL participate under the same geometric and damage rules

### Requirement: Area attacks hit every candidate once
An area attack SHALL apply its full damage independently to every eligible intersecting target, at most once per target per execution. Duplicate spatial entries SHALL NOT multiply hits, and damage SHALL NOT be divided by target count.

#### Scenario: Multiple targets and duplicate discovery
- **WHEN** three targets intersect and one is found through more than one spatial query
- **THEN** each target SHALL receive exactly one full hit from the execution

### Requirement: Single attacks choose the nearest intersection
A single-target attack SHALL choose one member of the same contact set as the area attack. Distance SHALL be measured from the sector origin to the nearest point of the collider portion inside the sector. Equal distances SHALL choose the lowest stable entity ID. The server SHALL choose at impact, without following a target selected at start.

#### Scenario: Center distance gives the wrong answer
- **WHEN** one target has the nearer center but another has the nearer collider intersection inside the sector
- **THEN** the second target SHALL receive the single hit

#### Scenario: Nearest collider point is outside the sector
- **WHEN** a collider's nearest overall point lies outside the sector
- **THEN** ranking SHALL use only its intersecting portion, not that point

#### Scenario: Stable tie
- **WHEN** two targets have equal intersection distance and enumeration order changes
- **THEN** the lower entity ID SHALL win both times

### Requirement: Melee damage preserves the approved formula
Raw damage SHALL equal `B × (STR/1)^0.25 × (Qweapon/10)^0.25 × multiplier`. Effective STR SHALL be read at impact and retain fractional precision. The equipped weapon chosen at start SHALL supply B and Quality. Combat-equipment locking SHALL preserve that weapon through the cycle. The action multiplier SHALL apply before armor. No global damage cap or minimum damage of 1 SHALL be introduced.

#### Scenario: Starter damage
- **WHEN** STR=1, weapon Quality=10, B=6, and target armor is zero
- **THEN** area damage SHALL be 6 and single-target damage SHALL be 9

#### Scenario: STR changes during windup
- **WHEN** STR changes from 1 to 16 before impact with the same weapon
- **THEN** raw damage SHALL double relative to STR=1 rather than use the old value

#### Scenario: Two equipped axes
- **WHEN** compatible axes with different Quality are equipped in both hands
- **THEN** the right-hand axe SHALL supply weapon parameters; a compatible left-hand axe SHALL be used only when the right hand has no compatible axe

### Requirement: Armor and receiver application retain fractional damage
Damage SHALL equal `Draw²/(Draw+A)` for positive Draw and zero for Draw=0, including A=0. A SHALL be nonnegative and read for each receiver at impact. The phase-1 range SHALL use A=0. Calculation and HP subtraction SHALL retain float64 precision without integer conversion, and invalid nonfinite inputs SHALL fail explicitly without partially applying a hit.

#### Scenario: Formula is ready for later armor
- **WHEN** the shared calculator receives Draw=6 and A=4, or Draw=9 and A=4
- **THEN** it SHALL return 3.6 or 81/13 respectively, while live range targets remain unarmored

#### Scenario: Zero damage
- **WHEN** Draw=0 and A=0
- **THEN** the result SHALL be exactly zero rather than NaN or a minimum positive damage

#### Scenario: Fractional HP
- **WHEN** a test receiver at HP=1 takes damage 0.4
- **THEN** its authoritative remaining HP SHALL be 0.6 without truncation and the next hit SHALL use that remainder

### Requirement: Test targets use the shared hit path
While `game.combat_test_enabled` is enabled, any authenticated player SHALL be allowed to create, reset, and remove their own range without an administrator role. Administrator roles are deferred. The opt-in test range SHALL provide server-owned receivers with HP and actual colliders, reset, stationary and moving fixtures, and authoritative HP/impact presentation. Their hits SHALL pass through the same targeting, calculation, and receiver-dispatch path intended for future objects and creatures. Test receiver health SHALL NOT invoke the current real-character KO/death path or mutate ordinary world-object health.

#### Scenario: Resettable demonstration
- **WHEN** an authenticated player creates the range in an enabled disposable test world
- **THEN** clients SHALL see repeatable fixtures including small, large, equal-distance, and moving targets, and resetting SHALL restore their configured HP and positions

#### Scenario: Receiver reaches zero
- **WHEN** damage reaches or exceeds a test target's remaining HP
- **THEN** HP SHALL clamp to zero, the target SHALL visibly become depleted and cease receiving hits until reset, without loot or real-world destruction side effects

#### Scenario: Range unavailable
- **WHEN** range/combat testing is disabled
- **THEN** the request SHALL fail without spawning fixtures, granting equipment, or activating combat test receivers

#### Scenario: Fresh fixture incarnation
- **WHEN** the range is removed and recreated
- **THEN** stale hits and updates from the old fixtures SHALL NOT affect the replacements

### Requirement: Runtime scope of phase one is explicit
Test fixtures and combat deadlines SHALL be runtime-only in phase 1. Range instructions SHALL identify missing persistence, disconnect protection, zone-transfer continuity, creature health, and world-object destruction. Persistent weapon instances and spent stamina SHALL continue using existing inventory/stat persistence. The range SHALL NOT be presented as completed persistent-world combat.

#### Scenario: Restart the test world
- **WHEN** the test server restarts
- **THEN** the range SHALL require recreation and SHALL NOT claim to restore combat deadlines or target HP; existing item Quality and spent stamina SHALL follow their existing save/load paths
