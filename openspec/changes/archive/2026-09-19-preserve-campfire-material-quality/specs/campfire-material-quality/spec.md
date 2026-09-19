# Spec Delta

## Purpose

Preserve the quality of branches used to build a campfire through its lifetime and ash outcome.

## ADDED Requirements

### Requirement: Campfire quality derives from consumed branch quality
The system SHALL set a completed campfire's quality to the arithmetic mean of the quality of every consumed `branch` input used to build it, weighted by consumed item count and rounded down to an integer. The calculation SHALL use every branch input even when a future recipe requires branches in multiple slots or stacks.

#### Scenario: Campfire built from multiple branch qualities
- **WHEN** a campfire is completed using branches with quality 3 and quality 8
- **THEN** the completed campfire SHALL have quality 5

#### Scenario: Repeated branch stack contributes by count
- **WHEN** a campfire is completed using two quality-4 branches and one quality-7 branch
- **THEN** the completed campfire SHALL have quality 5

### Requirement: Campfire ash retains constructed quality
The system SHALL create dropped `ash` with the quality of the exhausted campfire.

#### Scenario: Quality is preserved through exhaustion
- **WHEN** a quality-5 campfire exhausts and is replaced with ash
- **THEN** the resulting dropped ash SHALL have quality 5

### Requirement: Legacy campfires retain a safe fallback
The system SHALL leave a campfire at quality zero when its historical branch inputs are unavailable, including campfires persisted before material-quality calculation existed.

#### Scenario: Legacy campfire exhausts
- **WHEN** a legacy campfire with quality zero exhausts
- **THEN** the resulting dropped ash SHALL have quality zero
