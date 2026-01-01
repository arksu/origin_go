mmo-survival/
├── cmd/                          # Точки входа приложений
│   ├── gameserver/              # Game Server
│   │   └── main.go
│   ├── worldgen/                # Утилита генерации мира
│   │   └── main.go
│
├── internal/                     # Приватный код приложения
│   ├── gameserver/              # Логика Game Server
│   │   ├── server.go            # Основной server type
│   │   ├── config.go            # Конфигурация
│   │   └── metrics.go           # Метрики
│   │
│   ├── ecs/                     # Entity Component System
│   │   ├── world.go             # ECS World
│   │   ├── entity.go            # Entity manager
│   │   ├── query.go             # Query builder
│   │   ├── archetype.go         # Archetype-based storage
│   │   │
│   │   ├── components/          # Все компоненты
│   │   │   ├── transform.go
│   │   │   ├── movement.go
│   │   │   ├── collision.go
│   │   │   ├── health.go
│   │   │   ├── combat.go
│   │   │   ├── perception.go
│   │   │   ├── stealth.go
│   │   │   ├── inventory.go
│   │   │   ├── skills.go
│   │   │   ├── player.go
│   │   │   ├── bot.go
│   │   │   ├── static.go
│   │   │   ├── chunk_reference.go
│   │   │   ├── network_sync.go
│   │   │   └── farming.go
│   │   │
│   │   └── systems/             # Все системы
│   │       ├── system.go        # Базовый интерфейс System
│   │       ├── input_processing.go
│   │       ├── bot_ai.go
│   │       ├── movement.go
│   │       ├── collision.go
│   │       ├── interaction_range.go
│   │       ├── combat.go
│   │       ├── health.go
│   │       ├── visibility.go
│   │       ├── farming.go
│   │       ├── chunk_migration.go
│   │       └── synchronization.go
│   │
│   ├── world/                   # Управление миром
│   │   ├── chunk.go             # Структура чанка
│   │   ├── chunk_manager.go     # Менеджер чанков
│   │   ├── chunk_loader.go      # Загрузка/выгрузка
│   │   ├── chunk_serializer.go  # Сериализация
│   │   ├── layer.go             # Слои мира (surface, underground)
│   │   ├── tile.go              # Структура тайла
│   │   └── aoi.go               # Area of Interest
│   │
│   ├── spatial/                 # Пространственные структуры
│   │   ├── spatial_hash.go      # SpatialHash для чанков
│   │   ├── aabb.go              # AABB и операции
│   │   ├── swept_collision.go   # Swept AABB collision
│   │   ├── minkowski.go         # Minkowski difference
│   │   └── grid.go              # Grid для broad phase
│   │
│   ├── network/                 # Сетевая подсистема
│   │   ├── connection.go        # WebSocket connection wrapper
│   │   ├── connection_manager.go # Управление соединениями
│   │   ├── protocol/            # Protobuf protocol
│   │   │   ├── messages.proto   # Определения сообщений
│   │   │   ├── messages.pb.go   # Сгенерированный код
│   │   │   ├── encoder.go       # Encoding helpers
│   │   │   └── decoder.go       # Decoding helpers
│   │   ├── interest_manager.go  # Spatial Interest Management
│   │   ├── bandwidth.go         # Bandwidth management
│   │   └── compression.go       # Compression utilities
│   │
│   ├── persistence/             # Работа с БД
│   │   ├── database.go          # Database connection pool
│   │   ├── repository/          # Репозитории
│   │   │   ├── chunk_repo.go
│   │   │   ├── player_repo.go
│   │   │   ├── world_object_repo.go
│   │   │   └── farming_repo.go
│   │   ├── cache/               # Redis cache
│   │       ├── redis.go
│   │       ├── chunk_cache.go
│   │       ├── player_cache.go
│   │       └── visibility_cache.go
│   │
│   ├── game/                    # Игровая логика высокого уровня
│   │   ├── combat/              # Боевая система
│   │   │   ├── damage.go
│   │   │   ├── targeting.go
│   │   │   └── combat_resolver.go
│   │   ├── interaction/         # Взаимодействия
│   │   │   ├── interaction.go
│   │   │   ├── pickup.go
│   │   │   ├── use_object.go
│   │   │   └── trade.go
│   │   ├── inventory/           # Инвентарь
│   │   │   ├── inventory.go
│   │   │   ├── item.go
│   │   │   └── paperdoll.go
│   │   ├── skills/              # Система скиллов
│   │   │   ├── skill.go
│   │   │   ├── skill_tree.go
│   │   │   └── experience.go
│   │   ├── farming/             # Фарминг
│   │   │   ├── plant.go
│   │   │   ├── growth.go
│   │   │   └── harvest.go
│   │   └── weather/             # Погода
│   │       ├── weather.go
│   │       └── effects.go
│   │
│   ├── ai/                      # AI для ботов
│   │   ├── bot.go               # Bot controller
│   │   ├── behavior/            # Behavior Tree
│   │   │   ├── node.go
│   │   │   ├── selector.go
│   │   │   ├── sequence.go
│   │   │   └── decorators.go
│   │   ├── behaviors/           # Конкретные поведения
│   │   │   ├── patrol.go
│   │   │   ├── aggro.go
│   │   │   ├── flee.go
│   │   │   ├── farming.go
│   │   │   └── idle.go
│   │   └── lod.go               # Level of Detail для ботов
│   │
│   ├── events/                  # Event system
│   │   ├── event.go             # Event types
│   │   ├── event_bus.go         # Event bus
│   │   └── handlers/            # Event handlers
│   │       ├── combat_handler.go
│   │       ├── migration_handler.go
│   │       └── death_handler.go
│   │
│   └── utils/                   # Утилиты
│       ├── math/                # Математические функции
│       │   ├── vector.go
│       │   ├── rect.go
│       │   └── collision_math.go
│       ├── pool/                # Object pooling
│       │   ├── buffer_pool.go
│       │   ├── entity_pool.go
│       │   └── slice_pool.go
│       ├── time/                # Время и тики
│       │   ├── ticker.go
│       │   └── timer.go
│       └── logger/              # Логирование
│           └── logger.go
│
├── pkg/                         # Публичные библиотеки (можно использовать извне)
│   ├── common/                  # Общие типы и константы
│   │   ├── types.go
│   │   ├── constants.go
│   │   └── errors.go
│   │
│   ├── geom/                    # Геометрические примитивы
│   │   ├── vector2.go
│   │   ├── vector3.go
│   │   ├── bounds.go
│   │   └── transform.go
│   │
│   └── protocol/                # Публичные части протокола
│       └── definitions.go
│
├── api/                         # API definitions
│   ├── proto/                   # Protobuf files
│   │   ├── game.proto
│   │   ├── world.proto
│   │   └── admin.proto
│   └── rest/                    # REST API (для admin панели)
│       └── openapi.yaml
│
├── configs/                     # Конфигурационные файлы
│   ├── gameserver.yaml
│   ├── gateway.yaml
│   ├── database.yaml
│   └── game_balance.yaml        # Игровой баланс
│
├── scripts/                     # Утилитные скрипты
│   ├── setup_db.sh
│   ├── generate_proto.sh
│   ├── run_tests.sh
│   └── deploy.sh
│
├── migrations/                  # SQL миграции (альтернативное размещение)
│   └── postgres/
│       ├── 001_initial.up.sql
│       ├── 001_initial.down.sql
│       └── ...
│
├── test/                        # Интеграционные и E2E тесты
│   ├── integration/
│   │   ├── chunk_loading_test.go
│   │   ├── collision_test.go
│   │   └── combat_test.go
│   ├── e2e/
│   │   └── gameplay_test.go
│   └── fixtures/                # Тестовые данные
│       ├── chunks/
│       └── entities/
│
├── tools/                       # Development tools
│   ├── worldgen/                # Генератор мира
│   │   ├── generator.go
│   │   ├── noise.go
│   │   └── biomes.go
│   ├── profiling/               # Инструменты профилирования
│   │   └── profile.go
│   └── benchmark/               # Бенчмарки
│       └── ecs_bench_test.go
│
├── web/                         # Frontend (клиент)
│   ├── src/
│   │   ├── main.ts
│   │   ├── App.vue
│   │   ├── game/                # Игровая логика клиента
│   │   │   ├── Game.ts
│   │   │   ├── Renderer.ts
│   │   │   ├── InputManager.ts
│   │   │   └── NetworkClient.ts
│   │   ├── components/          # Vue компоненты
│   │   │   ├── GameCanvas.vue
│   │   │   ├── UI/
│   │   │   │   ├── Inventory.vue
│   │   │   │   ├── HealthBar.vue
│   │   │   │   └── SkillTree.vue
│   │   ├── pixi/                # PixiJS рендеринг
│   │   │   ├── EntityRenderer.ts
│   │   │   ├── TileRenderer.ts
│   │   │   └── effects/
│   │   ├── protocol/            # Protobuf для клиента
│   │   │   └── messages_pb.js
│   │   └── assets/              # Ассеты
│   │       ├── sprites/
│   │       ├── tilesets/
│   │       └── ui/
│   ├── public/
│   ├── package.json
│   ├── vite.config.ts
│   └── tsconfig.json
│
├── deployments/                 # Deployment конфиги
│   ├── docker/
│   │   ├── Dockerfile.gameserver
│   │   ├── Dockerfile.gateway
│   │   └── docker-compose.yml
│   ├── kubernetes/
│   │   ├── gameserver-deployment.yaml
│   │   ├── gateway-deployment.yaml
│   │   ├── postgres-statefulset.yaml
│   │   └── redis-deployment.yaml
│   └── terraform/               # Infrastructure as Code
│       └── main.tf
│
├── docs/                        # Документация
│   ├── architecture/
│   │   ├── overview.md
│   │   ├── ecs.md
│   │   ├── networking.md
│   │   └── spatial_systems.md
│   ├── api/
│   │   └── protocol.md
│   ├── game_design/
│   │   ├── combat.md
│   │   ├── farming.md
│   │   └── progression.md
│   └── deployment/
│       └── setup.md
│
├── .github/                     # GitHub workflows
│   └── workflows/
│       ├── test.yml
│       ├── build.yml
│       └── deploy.yml
│
├── go.mod
├── go.sum
├── Makefile                     # Команды для сборки/тестирования
├── README.md
└── .gitignore