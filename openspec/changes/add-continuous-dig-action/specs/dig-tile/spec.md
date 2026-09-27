# Spec Delta

## Purpose

Let players gather basic soil, clay, stone, and sand from eligible terrain through one targeted digging attempt that continues producing items while the player can receive and pay for them.

## ADDED Requirements

### Requirement: Dig targets specific terrain and produces its mapped item

The server SHALL provide an action with ID `dig` and label Dig in the Actions catalog. It SHALL use a tile target, the existing `dig` cursor and icon, tile-center approach, no skill or equipment requirement, a 20-tick cycle, a fixed 300-stamina cost per successful cycle, and `execution.repeat: true`. It SHALL not require a new click between successful cycles on the same selected tile. It SHALL not return to target selection after the repeated attempt ends.

Only Grass, Shallow Water, Mountain, and Sand terrain SHALL be valid dig targets. A successful cycle SHALL grant exactly one item of quality 10 according to this mapping: Grass to soil, Shallow Water to clay, Mountain to stone, and Sand to sand. It SHALL leave the target terrain, its tile version, and all neighboring tiles unchanged. An unloaded target tile SHALL be reported as unavailable; loaded ineligible terrain SHALL be rejected as bad terrain. Object presence alone SHALL not reject a tile, but a collider that prevents reaching its center MAY prevent the attempt.

#### Scenario: Each eligible terrain grants the mapped item
- **WHEN** a player completes a dig cycle on Grass, Shallow Water, Mountain, or Sand
- **THEN** the player SHALL receive exactly one Q10 soil, clay, stone, or sand item respectively, and no tile SHALL change

#### Scenario: Ineligible or unloaded terrain
- **WHEN** the player clicks an ineligible loaded tile or a tile whose chunk is unavailable
- **THEN** the server SHALL alert, SHALL NOT start an approach or cycle, and SHALL NOT grant an item or charge action stamina

#### Scenario: Object over target tile
- **WHEN** an eligible tile has an object that does not block arrival at the tile center
- **THEN** the action SHALL target the clicked tile rather than interact with that object

### Requirement: Dig approaches the target center and repeats while valid

One accepted click SHALL identify one tile regardless of click position. The player SHALL approach that tile's center using the existing tile-center arrival tolerance; the first 20-tick cycle SHALL start only after actual arrival. After each successful nonterminal cycle, another 20-tick cycle SHALL begin on that same tile without movement or a new click, provided its current terrain remains eligible, the player is still within arrival tolerance, and at least 300 stamina remains. The server SHALL verify current terrain and the player's actual position before granting each item; if the terrain changed to another eligible type, the current type SHALL determine the next item. A blocked, canceled, or timed-out approach, departure from the target center at completion, or a change to ineligible terrain SHALL stop the attempt without a new item or action stamina charge for the unfinished cycle.

#### Scenario: Continuous gathering from unchanged terrain
- **WHEN** the player arrives at an eligible tile center and completes two consecutive cycles while terrain, position, stamina, and inventory remain valid
- **THEN** the player SHALL receive two mapped items, lose exactly 600 action stamina, and remain in an executing cycle on that tile without a second click

#### Scenario: Movement away during a cycle
- **WHEN** the player is outside arrival tolerance at a cycle's completion check
- **THEN** that cycle SHALL stop without an item or action stamina charge

#### Scenario: Target terrain becomes ineligible
- **WHEN** another action changes the selected tile to ineligible terrain after digging begins
- **THEN** the attempt SHALL stop without granting an item or charging action stamina for the unfinished cycle

### Requirement: Dig uses standard player item delivery and stops at capacity

Each dig output SHALL use the standard player item grant behavior: try the player's inventory, including eligible nested containers, and then the free hand. A successful grant SHALL be reflected in the player's inventory and discovery updates. If the item is granted to the hand, that cycle SHALL complete and cost 300 stamina, then digging SHALL stop immediately. If no eligible inventory space or hand slot exists, or granting fails, the attempt SHALL stop with an alert and SHALL grant no item or charge action stamina for that cycle. Dig SHALL never drop its output to the world as a capacity fallback.

#### Scenario: Inventory receives the item
- **WHEN** the player has eligible inventory space at cycle completion
- **THEN** one item SHALL be granted using the standard inventory placement order and digging SHALL continue if all other conditions still hold

#### Scenario: Hand receives the last item
- **WHEN** inventory placement is unavailable but the hand is free at cycle completion
- **THEN** one item SHALL be granted to the hand, exactly 300 action stamina SHALL be charged, and digging SHALL stop without starting another cycle

#### Scenario: No destination for an item
- **WHEN** no eligible inventory space or free hand exists when a cycle attempts to grant its item
- **THEN** digging SHALL stop with an alert and SHALL not grant an item, drop one on the ground, or charge action stamina for that cycle

### Requirement: Dig charges only successful cycles and supports cancellation

The server SHALL require at least 300 stamina before beginning the approach, at the start of each cycle, and immediately before granting an item. Each successful item grant SHALL cost exactly 300 action stamina regardless of maximum stamina or attributes. If stamina is insufficient, the attempt SHALL stop and the current cycle SHALL grant no item or charge action stamina. Normal movement stamina costs MAY still apply during approach. Escape, a secondary map click, switching actions, and interruption SHALL cancel ongoing digging without another item or action stamina charge, and SHALL reset the cursor. A late completion from a canceled attempt SHALL not grant an item.

#### Scenario: Stamina runs out between cycles
- **WHEN** a completed cycle leaves fewer than 300 stamina units
- **THEN** its item and 300-stamina charge SHALL remain, but no following cycle SHALL start

#### Scenario: Stamina becomes insufficient during a cycle
- **WHEN** stamina falls below 300 before that cycle completes
- **THEN** digging SHALL stop without an item or action stamina charge for that cycle

#### Scenario: Player cancels a cycle
- **WHEN** the player cancels digging before a cycle completes
- **THEN** the server SHALL stop the cycle, grant no item, charge no action stamina for it, and reset the cursor
