create schema if not exists origin;
set search_path to origin;

-- ACCOUNT -------------------------------------------------------------

CREATE TABLE IF NOT EXISTS account
(
    id             BIGSERIAL PRIMARY KEY,
    login          VARCHAR(128) UNIQUE NOT NULL,
    password_hash  text                NOT NULL,
    status         SMALLINT    DEFAULT 0, -- 0=active, 1=banned, 2=suspended
    token          text unique,
    last_logged_at TIMESTAMPTZ         NULL,
    created_at     TIMESTAMPTZ DEFAULT now(),
    updated_at     TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX idx_account_status ON account (status) WHERE status != 0;
-- для аналитики активных
CREATE INDEX idx_account_last_logged ON account (last_logged_at);

-- CHARACTER (player) -----------------------------------------------------------

CREATE TABLE IF NOT EXISTS character
(
    id               bigint PRIMARY KEY,
    account_id       BIGINT       NOT NULL REFERENCES account (id) ON DELETE CASCADE,
    name             VARCHAR(128) NOT NULL,

    region           INT          NOT NULL,                       -- region id (continent)
    x                INT          NOT NULL,                       -- world coordinate
    y                INT          NOT NULL,                       -- world coordinate
    layer            INT          NOT NULL,                       -- ground layer
    heading          SMALLINT     NOT NULL CHECK (heading >= 0 AND heading < 360),

    stamina          NUMERIC      NOT NULL CHECK (stamina >= 0),  -- current stamina
    energy           NUMERIC      not null check ( energy >= 0 ), -- current energy
    shp              INT          NOT NULL CHECK (shp >= 0),      -- soft health points
    hhp              INT          NOT NULL CHECK (hhp >= 0),      -- hard health points

    attributes       JSONB        NOT NULL,

    -- Опыт (денормализация для быстрого доступа)
    exp_nature       BIGINT                DEFAULT 0,
    exp_industry     BIGINT                DEFAULT 0,
    exp_combat       BIGINT                DEFAULT 0,

    online_time      BIGINT       NOT NULL DEFAULT 0,             -- time in seconds spent in game
    auth_token       VARCHAR(64),                                 -- token used in C2SAuth packet
    token_expires_at TIMESTAMPTZ,
    is_online        BOOLEAN               DEFAULT false,

    -- Для disconnect логики
    disconnect_at    TIMESTAMPTZ,                                 -- когда отключился
    is_ghost         BOOLEAN               DEFAULT false,         -- персонаж остался в мире после disconnect

    last_save_at     TIMESTAMPTZ,
    deleted_at       TIMESTAMPTZ,                                 -- when delete set now()
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ           DEFAULT now()
);
CREATE INDEX idx_character_account ON character (account_id) WHERE deleted_at IS NULL;
CREATE unique index idx_character_auth_token ON character (auth_token)
    WHERE auth_token IS NOT NULL AND deleted_at IS NULL;
-- для очистки ghost персонажей
CREATE INDEX idx_character_ghost ON character (disconnect_at)
    WHERE is_ghost = true;
-- Настройка autovacuum для горячих таблиц
ALTER TABLE character
    SET (
        autovacuum_vacuum_scale_factor = 0.05,
        autovacuum_analyze_scale_factor = 0.02,
        autovacuum_vacuum_cost_delay = 5
        );

-- INVENTORY -----------------------------------------------------------

CREATE TABLE IF NOT EXISTS inventory
(
    id            BIGSERIAL PRIMARY KEY,
    owner_id      BIGINT   NOT NULL, -- character_id
    kind          smallint not null, -- 0=grid, 1=hand, 2=equipment 3=machine_input, 4=machine_output
    inventory_key smallint NOT NULL, -- уникальный ID контейнера (внутри одного объекта может быть несколько инвентарей)
    data          JSONB    NOT NULL, -- содержимое контейнера (inventory)
    updated_at    TIMESTAMPTZ DEFAULT now(),
    deleted_at    TIMESTAMPTZ,
    version       integer  not null,

    UNIQUE (owner_id, kind, inventory_key)
);
CREATE INDEX idx_inventory_owner_active ON inventory (owner_id, kind, inventory_key)
    WHERE deleted_at IS NULL;

-- CHUNK ----------------------------------------------------------------
-- when load - also load objects by grid_x, grid_y, use (grid_x, grid_y) in spatial hash grid
CREATE TABLE IF NOT EXISTS chunk
(
    region        INT    NOT NULL,       -- region id
    x             INT    NOT NULL,       -- chunk x coordinate
    y             INT    NOT NULL,       -- chunk y coordinate
    layer         INT    NOT NULL,       -- ground layer
    version       INT         DEFAULT 1, -- для оптимистичных блокировок
    last_tick     BIGINT NOT NULL,
    last_saved_at TIMESTAMPTZ DEFAULT now(),
    tiles_data    BYTEA  NOT NULL,
    entity_count  INT         DEFAULT 0, -- количество объектов в чанке
    PRIMARY KEY (region, x, y, layer)
);
create index idx_chunk on chunk (region, x, y, layer);
-- для tiles_data
ALTER TABLE chunk
    ALTER COLUMN tiles_data SET COMPRESSION lz4;

-- OBJECT --------------------------------------------------------------
-- object in grid, keep actual grid_x, grid_y when object moving, load by grid_x, grid_y
CREATE TABLE IF NOT EXISTS object
(
    id          BIGINT,
    type_id     INT    NOT NULL, -- defId from object definitions

    region      INT    NOT NULL,
    x           INT    NOT NULL, -- world coordinate
    y           INT    NOT NULL, -- world coordinate
    layer       INT    NOT NULL,
    chunk_x     INT    NOT NULL, -- redundant data for loading by chunk
    chunk_y     INT    NOT NULL, -- redundant data for loading by chunk
    heading     SMALLINT CHECK (heading >= 0 AND heading < 360),

    quality     SMALLINT CHECK (quality >= 0) not null default 10,
    hp          INT CHECK (hp >= 0),

    owner_id    BIGINT,          -- кто создал/владеет (для построек)
    data        JSONB,

    created_at  TIMESTAMPTZ DEFAULT now(),
    create_tick BIGINT NOT NULL,
    last_tick   BIGINT NOT NULL,
    updated_at  TIMESTAMPTZ DEFAULT now(),
    deleted_at  TIMESTAMPTZ,     -- soft delete для аудита
    PRIMARY KEY (region, id)
) PARTITION BY LIST (region);
CREATE TABLE object_region_1 PARTITION OF object FOR VALUES IN (1);
CREATE TABLE object_region_2 PARTITION OF object FOR VALUES IN (2);

-- Критически важные индексы
-- для загрузки чанков
CREATE INDEX idx_object_chunk ON object (region, chunk_x, chunk_y, layer)
    WHERE deleted_at IS NULL;
-- для spatial queries
CREATE INDEX idx_object_spatial ON object (region, x, y, layer)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_object_data ON object USING GIN (data)
    WHERE data IS NOT NULL;
ALTER TABLE object
    ALTER COLUMN region SET STATISTICS 1000;
ALTER TABLE object
    ALTER COLUMN chunk_x SET STATISTICS 1000;
ALTER TABLE object_region_1
    SET (
        autovacuum_vacuum_scale_factor = 0.1,
        autovacuum_analyze_scale_factor = 0.05
        );

-- SKILL ---------------------------------------------------------------

CREATE TABLE IF NOT EXISTS skill
(
    character_id BIGINT NOT NULL REFERENCES character (id) ON DELETE CASCADE,
    skill_id     INT    NOT NULL,
    experience   BIGINT      DEFAULT 0 CHECK (experience >= 0),
    level        INT    NOT NULL,

    last_gain_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ DEFAULT now(),
    updated_at   TIMESTAMPTZ DEFAULT now(),

    PRIMARY KEY (character_id, skill_id)
);
CREATE INDEX idx_skill_character ON skill (character_id);

-- CHAT HISTORY --------------------------------------------------------

CREATE TABLE IF NOT EXISTS chat
(
    id          BIGSERIAL,
    channel     SMALLINT    NOT NULL,
    sender_id   BIGINT      NOT NULL REFERENCES character (id),
    receiver_id BIGINT,               -- для whisper

    region      INT         NOT NULL,
    x           INT         NOT NULL, -- where was said (object coord)
    y           INT         NOT NULL,
    layer       SMALLINT    NOT NULL,

    message     text        NOT NULL CHECK (length(message) > 0 AND length(message) <= 1200),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);
CREATE TABLE chat_default
    PARTITION OF chat DEFAULT;

CREATE INDEX idx_chat_channel ON chat (channel, created_at DESC);
CREATE INDEX idx_chat_whisper ON chat (channel, sender_id, receiver_id, created_at DESC);
CREATE INDEX idx_chat_sender ON chat (channel, sender_id, created_at DESC);
CREATE INDEX idx_chat_region ON chat (channel, region, created_at DESC);

-- GUILD SITE ----------------------------------------------------------

CREATE TABLE if not exists build_site
(
    id                    BIGSERIAL PRIMARY KEY,
    object_id             BIGINT unique NOT NULL,
    recipe_id             INT           NOT NULL,
    builder_id            BIGINT REFERENCES character (id),

    -- Прогресс
    build_points_total    INT           NOT NULL CHECK (build_points_total > 0),
    build_points_provided INT         DEFAULT 0 CHECK (build_points_provided >= 0),
    build_points_current  INT         DEFAULT 0 CHECK (build_points_current >= 0),

    -- Состояние
    status                SMALLINT    DEFAULT 0,  -- 0=in_progress, 1=completed, 2=cancelled

    -- Требуемые ресурсы (JSONB)
    required_items        JSONB         NOT NULL, -- [{"item_id": 1, "quantity": 10, "provided": 5}, ...]

    started_at            TIMESTAMPTZ DEFAULT now(),
    completed_at          TIMESTAMPTZ,

    CHECK (build_points_current <= build_points_provided),
    CHECK (build_points_provided <= build_points_total)
);

CREATE INDEX idx_build_site_status ON build_site (status, started_at);
CREATE INDEX idx_build_site_object ON build_site (object_id);

-- GLOBAL VAR ----------------------------------------------------------
-- used for runtime configuring game server and store global game data (server time, etc)
CREATE TABLE IF NOT EXISTS global_var
(
    name         VARCHAR(32) PRIMARY KEY,
    value_long   BIGINT,
    value_string VARCHAR(1024)
);
