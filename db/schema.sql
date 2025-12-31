create schema if not exists origin;
set search_path to origin;

-- ACCOUNT -------------------------------------------------------------

CREATE TABLE IF NOT EXISTS account
(
    id             BIGSERIAL PRIMARY KEY,
    login          VARCHAR(128) UNIQUE NOT NULL,
    password       VARCHAR(128)        NOT NULL,
    email          VARCHAR(128),
    last_logged_at TIMESTAMPTZ         NULL,
    created_at     TIMESTAMPTZ DEFAULT now()
);

-- CHARACTER (player) -----------------------------------------------------------

CREATE TABLE IF NOT EXISTS character
(
    id          BIGINT PRIMARY KEY,
    account_id  BIGINT       NOT NULL REFERENCES account (id),
    name        VARCHAR(128) NOT NULL,

    region      INT          NOT NULL, -- region id (continent)
    x           INT          NOT NULL, -- world coordinate
    y           INT          NOT NULL, -- world coordinate
    layer       INT          NOT NULL, -- ground layer
    heading     SMALLINT     NOT NULL, -- rotating angle (8 angles, 45 degrees)

    stamina     INT          NOT NULL, -- current stamina
    shp         INT          NOT NULL, -- soft health points
    hhp         INT          NOT NULL, -- hard health points

    online_time BIGINT       NOT NULL, -- time in seconds spent in game
    auth_token  text         null unique, -- token used in C2SAuth packet
    deleted_at  TIMESTAMPTZ  NULL, -- when delete set now()
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- INVENTORY -----------------------------------------------------------

CREATE TABLE IF NOT EXISTS inventory
(
    id           BIGINT PRIMARY KEY,
    parent_id    BIGINT   NOT NULL,
    item_type    INT      NOT NULL,
    slot_x       INT      NOT NULL,
    slot_y       INT      NOT NULL,
    item_quality SMALLINT NOT NULL,
    item_count   INT      NOT NULL,
    last_tick    BIGINT   NOT NULL,
    data_hex     VARCHAR(254), -- binary data converted to string hex data in uppercase
    deleted      BOOLEAN  NOT NULL
);

-- CHUNK ----------------------------------------------------------------
-- when load - also load objects by grid_x, grid_y, use (grid_x, grid_y) in spatial hash grid
CREATE TABLE IF NOT EXISTS chunk
(
    region    INT    NOT NULL, -- region id
    x         INT    NOT NULL, -- chunk x coordinate
    y         INT    NOT NULL, -- chunk y coordinate
    layer     INT    NOT NULL, -- ground layer
    last_tick BIGINT NOT NULL,
    data      BYTEA  NOT NULL
);
create index idx_chunk on chunk (region, x, y, layer);

-- OBJECT --------------------------------------------------------------
-- object in grid, keep actual grid_x, grid_y when object moving, load by grid_x, grid_y
CREATE TABLE IF NOT EXISTS object
(
    id          BIGINT PRIMARY KEY,
    region      INT      NOT NULL,
    x           INT      NOT NULL, -- world coordinate
    y           INT      NOT NULL, -- world coordinate
    layer       INT      NOT NULL,
    heading     SMALLINT NOT NULL,
    chunk_x     INT      NOT NULL, -- redundant data for loading by chunk
    chunk_y     INT      NOT NULL, -- redundant data for loading by chunk
    type_id     INT      NOT NULL, -- type id
    quality     SMALLINT NOT NULL,
    hp          INT      NOT NULL,
    create_tick BIGINT   NOT NULL,
    last_tick   BIGINT   NOT NULL,
    data_hex    VARCHAR(254)       -- binary data converted to string hex data in uppercase
);

CREATE INDEX idx_object ON object (region, x, y, layer);

-- GLOBAL VAR ----------------------------------------------------------
-- used for runtime configuring game server and store global game data (server time, etc)
CREATE TABLE IF NOT EXISTS global_var
(
    name         VARCHAR(32) PRIMARY KEY,
    value_long   BIGINT,
    value_string VARCHAR(1024)
);

-- CHAT HISTORY --------------------------------------------------------

CREATE TABLE IF NOT EXISTS chat
(
    channel    SMALLINT      NOT NULL,
    sender_id  BIGINT        NOT NULL,
    region     INT           NOT NULL,
    x          INT           NOT NULL, -- where was said (object coord)
    y          INT           NOT NULL,
    layer      INT           NOT NULL,
    text       VARCHAR(1020) NOT NULL,
    created_at TIMESTAMPTZ   NOT NULL DEFAULT now()
);

-- SKILL ---------------------------------------------------------------

CREATE TABLE IF NOT EXISTS skill
(
    id           bigserial PRIMARY KEY,
    character_id BIGINT references character (id),
    skill_id     INT NOT NULL,
    level        INT NOT NULL
);
