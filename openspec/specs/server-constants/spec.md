# server-constants Specification

## Purpose

Provide each authenticated connection with one server-owned snapshot of immutable world, movement, and calendar constants before world bootstrap, without duplicating those values in client code or world-entry packets.

## Requirements

### Requirement: Server constants have one immutable wire representation
The server SHALL provide one `S2C_ServerConstants` payload containing `coord_per_tile`, `chunk_size`, `tick_rate`, `directional_movement_supported`, `real_seconds_per_game_day`, `hours_per_day`, `days_per_month`, and `months_per_year`. The numeric values SHALL be positive integers and SHALL reflect the actual server world, loop, and calendar rules. Boolean false SHALL be a valid directional support value. Values SHALL remain unchanged throughout the server run and SHALL be the same for all connections and layers of that run. The packet SHALL NOT contain current runtime, entity state, stream epochs, player audio parameters, or derived calendar periods.

#### Scenario: Server calendar and packet use the same constants
- **WHEN** the server uses a 28,800-second game day, 24 hours per day, 30 days per month, and 12 months per year
- **THEN** the constants packet contains those same calendar values together with the actual world and movement constants
- **AND** clients derive calendar periods and speed from those received values

#### Scenario: Directional movement is disabled
- **WHEN** a valid constants packet declares `directional_movement_supported` false
- **THEN** the client accepts the constants and keeps directional input disabled

### Requirement: Constants are sent once in connection bootstrap order
After successful authentication the server SHALL enqueue `AuthResult(success)`, then one `S2C_ServerConstants`, before starting world bootstrap or replying to subsequent Pings from that connection. Both mandatory bootstrap packets SHALL use delivery that either enqueues the message or ends the connection; they SHALL NOT be silently dropped. Failure to prepare, serialize, or enqueue them SHALL close the connection, abort bootstrap, and allow existing disconnect cleanup to clear the authenticated character's online state. Repeated authentication on an already authenticated connection SHALL NOT establish another identity or resend constants. Failed authentication SHALL NOT send constants. No client request, acknowledgment, revision, refresh timer, or runtime-update protocol SHALL be added for constants.

#### Scenario: First successful connection
- **WHEN** a connection successfully authenticates and sends its immediate Ping
- **THEN** the client receives authentication success, then one constants packet, before its runtime Pong and any world bootstrap

#### Scenario: Transfer and additional Pings
- **WHEN** an authenticated connection transfers between layers or sends periodic or additional Pings
- **THEN** no further constants packets are sent
- **AND** runtime continues to refresh through Pong

#### Scenario: Reconnect
- **WHEN** the client establishes a new authenticated connection
- **THEN** that new connection receives one fresh constants packet

#### Scenario: Mandatory bootstrap cannot be prepared or enqueued
- **WHEN** the server cannot prepare, serialize, or enqueue authentication success or constants after marking the character online
- **THEN** it ends the connection and does not start world bootstrap
- **AND** existing disconnect cleanup clears the online state so a subsequent login can succeed

### Requirement: World entry carries only per-entry state for migrated fields
`coord_per_tile`, `chunk_size`, `tick_rate`, and `directional_movement_supported` SHALL be removed from `S2C_PlayerEnterWorld` and supplied only by server constants. Their old field numbers 3, 4, 5, and 10 and their names SHALL be reserved rather than reused. Remaining entity identity, name, stream epoch, and audio fields SHALL retain their existing numbers and semantics. The server SHALL NOT duplicate migrated constants in world-entry packets for compatibility.

#### Scenario: Entry and layer transfer
- **WHEN** a player enters or transfers into a world
- **THEN** the entry message carries that entry's entity identity, name, epoch, and audio state
- **AND** the client obtains geometry, tick rate, and movement support from its connection's constants

### Requirement: Client requires connection constants without numeric fallbacks
The client SHALL validate and retain a read-only constants snapshot for the current authenticated connection. It SHALL NOT embed fallback values for the transmitted constants or require received values to equal current server defaults. World/bootstrap processing, parameter-dependent rendering and coordinate calculations, and movement input SHALL require valid constants. Renderer initialization before connection SHALL remain safe while constants are unavailable. The snapshot SHALL survive world entry, leave, and layer transfers, and SHALL clear on a new connection, disconnect, or connection error. Late messages from a retired connection SHALL NOT install constants. An identical duplicate snapshot SHALL leave state unchanged; different constants in the same connection SHALL be a protocol error, not a live reconfiguration.

#### Scenario: Rendering starts before authentication
- **WHEN** the renderer initializes before valid constants arrive
- **THEN** parameter-dependent world calculations remain inactive without throwing or producing invalid coordinates
- **AND** no client numeric defaults stand in for server constants

#### Scenario: World entry arrives without valid constants
- **WHEN** a client receives world bootstrap without a valid constants snapshot
- **THEN** it enters the existing connection-error path rather than rendering or accepting movement with guessed values

#### Scenario: A different valid server profile on reconnect
- **WHEN** a new connection supplies different valid world, movement, or calendar constants
- **THEN** the client uses those received values for that connection
- **AND** no old connection's constants or readiness are reused

#### Scenario: Runtime changes are not supported
- **WHEN** another constants packet on the same connection changes a value
- **THEN** the client treats it as a protocol error and does not update the active constants

### Requirement: Bootstrap migration is deployed with matching peers
Moving constants out of world entry SHALL be treated as a breaking bootstrap change requiring matching server and client versions. Removed fields and names SHALL remain reserved, and unrelated protocol numbers SHALL remain unchanged. The additive runtime Pong field SHALL NOT be used to claim mixed-version compatibility for the complete change.

#### Scenario: Old server has no constants packet
- **WHEN** a new client receives a legacy world bootstrap without server constants
- **THEN** it rejects bootstrap instead of relying on old field locations or numeric defaults
- **AND** its existing wall-time Ping/Pong calculation remains independent of calendar readiness
