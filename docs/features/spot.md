Первые результаты уже дали важные зацепки: в H&H это называется «quality nodes» (качественные узлы), а «spotting» — это ещё и механика обнаружения ресурсов игроком (Perception/Exploration). Продолжаю: форумные треды и вики Ring of Brodgar (там формулы).

Ключевая механика «spotting» уже ясна (PER×EXP, случайная сложность от base до 4×base на каждого игрока). Теперь копаю «quality nodes» — как генерируются сами источники качества. Fandom вернул 403, попробую через другой ридер.**🌐 Z.ai Built-in Tool: webReader**


Отличные данные по шахтам (3 типа узлов: рудные жилы, качество породы, твёрдость — все размещены случайно и независимо). Теперь достану полные форумные треды про quality nodes и поищу детали генерации мира.

Картина узлов проясняется (пиковая клетка + спад качества по кругу, истощение при сборе, регенерация). Остались: генерация мира, рыбные споты, спавн животных, вода. Продолжаю.**🌐 Z.ai Built-in Tool: webReader**

Нашёл важное: деревья расставляются кластерами на фазе генерации мира, есть тред «Map generator - how does it work?» и упоминание «prespawned mining points». Копаю глубже.

Есть след: loftar прикладывал `patch.py` — генератор «patch»-форм (наверняка те самые пятна-споты), но не раскрыл правила расстановки, чтобы не убивать исследование мира. Попробую вытащить сам файл и продолжаю по рыб/животным.**🌐 Z.ai Built-in Tool: webReader**


Рыба — тоже локализованный ресурс-узел («working in the same way as any other localized resource» — jorb). И всплыло важное: в ноябре 2025 вышло «Prelude: World 16.1» с заявлением «Raw resources should have no Quality until processed» — похоже, систему quality-нод меняют. Попробую вытащить patch.py правильно и дочитать треды про спавн животных.

Ключевая находка по животным: спавн-точки фиксированы, качество привязано к quality-нодам (до +500%), респавн по циклу при перезагрузке чанков. Теперь скачаю patch.py (id=557) и найду тред World 16.1.

patch.py скачан — это настоящий код loftar'а для генерации органических «пятен». Дочитаю его целиком и параллельно найду тред World 16.1.

patch.py разобран: это генератор органических «блобов» — ветвящееся дерево точек → сплайн-контур → заливка (patch = пятно местности/спота). Теперь найду тред World 16.1 и поищу официальные заявления разработчиков про quality nodes.

Крупная находка в Prelude World 16: «quality noise fields of foragables and animals drift every new moon» — то есть узлы качества реализованы как шумовые поля по карте, дрейфующие каждый новый месяц! Достаю эти секции анонса целиком.

Почти всё собрано. Осталось проверить прелюдии World 13/14 — там могла появиться система «quests refresh nodes» — и вики-статью про поиск высококачественных нод.

Информация собрана — картина полная. Сохраню результаты исследования в память и подведу итог.

Исследование завершено — собрал всё из вики (Ring of Brodgar, Fandom), официального форума (включая прямые заявления loftar и jorb) и даже вытащил исходник генератора мира `patch.py`, который loftar прикладывал на форуме. Сохранено в память проекта.

# TL;DR

В Haven & Hearth «споты» — это **quality nodes**: локальные максимумы скрытых **шумовых полей качества**, отдельных для каждого типа ресурса. Слово «Spot» в игре — это ещё и отдельная механика *обнаружения* ресурса игроком (Perception × Exploration). Качество источника задаётся узлом при генерации мира, падает от сбора и (после World 16) дрейфует со временем.

# Официальная модель (от разработчиков)

Единственное прямое официальное описание — в [Prelude: World 16](https://www.havenandhearth.com/forum/viewtopic.php?f=39&t=76519) (jorb, окт 2024):

> **«The quality noise fields of foragables and animals drift every new moon, with animal quality drifting faster than herb quality.»**

То есть технически: для каждого типа ресурса (каждой травы, каждого вида животных) есть своё непрерывное шумовое поле качества поверх координат мира. «Узел» — это просто локальный максимум этого поля. Каждую «новую луну» (~месяц) поля дрейфуют, узлы перемещаются. Там же: «Quests no longer yield local quality increases as a potential reward» — раньше квесты могли *поднимать* качество узлов локально (ввели ~W13–14, в W16 убрали).

loftar о генераторе мира ([тред](https://www.havenandhearth.com/forum/viewtopic.php?f=7&t=11222)): генератор написан на **Python**, пятна местности рисуются через **libgd**, и правила расстановки он сознательно не публикует — «it might potentially reveal some secrets that can currently only be found by exploring». Но сам **`patch.py`** он приложил — я скачал его с форума (лежит в `/tmp/patch.py`, 224 строки). Это генератор органических «блобов»:
- `gentree()` — случайное ветвящееся дерево точек (углы/радиусы случайные);
- `genoutline()` — вокруг ветвей строится сплайн-полигон с разбросом радиуса ±20%;
- `draw()` — заливка `fillToBorder` + случайные мелкие кляксы вдоль контура.

Так выглядят и пятна местности, и, судя по «prespawned mining points» из треда о сбросе мира 2009 года, области спотов. **Где** ставить пятна — решает невыпущенная часть генератора.

# Структура узла (наблюдения игроков, [t=13760](https://www.havenandhearth.com/forum/viewtopic.php?t=13760))

- У узла есть **пиковая клетка в центре**, качество **спадает по кругу** от центра.
- Чем выше пик, тем **больше радиус** (Q100-узел находить проще, чем Q15) — подтверждено игроками, но точная формула неизвестна.
- Узел **не обязан быть в «правильной» местности**: глиняный узел может лежать не в mudflats, а на суше; «большая часть земли вообще без глиняных узлов».
- Узлы бывают «comically small» — падение с 80 до 60 за 20 тайлов ([t=69940](https://www.havenandhearth.com/forum/viewtopic.php?t=69940)).

# Правила качества по типам источников

| Ресурс | Как задаётся качество |
|---|---|
| **Травы/сбор** | У каждой травы свои узлы; собранное Q = узел, **жёстко ограничено Survival** собирающего ([Foraging](https://ringofbrodgar.com/wiki/Foraging)) |
| **Руда/камень** | Под землёй **3 независимых случайных типа узлов**: рудные жилы, качество породы, твёрдость — «placed randomly and have no relation to each other». Добытое Q = узел качества породы этой области (иначе base); с каждым уровнем вниз base Q и твёрдость растут. Хардкап — Masonry, софткап — Q инструмента; кредо применяются *после* хардкапа. С 2023 разброс качества — на клетку, а не на кусок ([Mining](https://ringofbrodgar.com/wiki/Mining)) |
| **Вода** | Колодец пробивает карту водного стола: Q тем выше, чем ближе к центру подземного источника; **не ограничено навыками**; ищется лозоходством («the table will rise no further from here») ([Well](https://ringofbrodgar.com/wiki/Well)) |
| **Глина/почва** | «Единственные ресурсы, чьё качество игрок поднять не может» — только найти источник. Q почвы читается по травам, растущим на клетке ([гайд Fandom](https://havenandhearth.fandom.com/wiki/Finding_high_quality_water,_clay,_and_soil)) |
| **Рыба** | Тоже узлы — jorb: «Fish are now a limited and localized resource, working in the same way as any other localized resource». Вид рыбы случаен от узла; **у каждого вида своя оптимальная точка внутри узла** (иди туда, где шанс клёва растёт — «аналогично поиску максимума качества в узлах трав»); Q хардкапится Survival ([Fishing](https://ringofbrodgar.com/wiki/Fishing)) |
| **Животные** | **Фиксированные точки спавна с фиксированным Q**, привязанные к quality-нодам — буст «up to +500% depending on the node» ([t=74397](https://www.havenandhearth.com/forum/viewtopic.php?t=74397)). Спавн срабатывает при перезагрузке чанков (проверка ~раз в 15 минут), стадо появляется компактно и разбегается ([t=10431](https://www.havenandhearth.com/forum/viewtopic.php?t=10431)) |
| **Деревья** | Расставляются на фазе генерации мира кластерами по биомам + часть просто случайно, без привязки. Дикое дерево всегда **Q10**; посаженное = Q семени пополам, минимум 10 ([Tree](https://ringofbrodgar.com/wiki/Tree)) |

# Динамика: истощение и регенерация

- Сбор **постепенно истощает узел**: качество падает; «когда должно упасть ниже 10, он просто перестаёт давать что-либо» — до восстановления ([t=13760](https://www.havenandhearth.com/forum/viewtopic.php?t=13760)).
- Глина/почва/вода: восстановление «за несколько дней, если оставить в покое».
- W16: поля трав/животных дрейфуют каждый месяц (животные быстрее) — анти-бот мера против «замингованных» узлов.
- Поверх узлов идут бонусы: реалм-баффы (Woodland +6% на лесных животных/сбор, Marriage of the Sea +15% на рыбу), у урожая — «geographically local variance».

# Механика Spot (обнаружение)

Отдельно от узлов — *видимость* ресурсов ([Foraging](https://ringofbrodgar.com/wiki/Foraging)):
- Оценка = **Perception × Exploration**. У каждого собираемого предмета есть base PER×EXP-порог; минимальный порог = ½ base (начинаешь находить), 2× base (гарантированно видишь всё).
- Сложность конкретного экземпляра **пересчитывается случайно (от base до 4×base) отдельно для каждого персонажа при каждом попадании в радиус зрения** — трава может «мигать» видимостью.
- Примеры порогов: черника 10, крапива 50, Rustroot 1000, Эдельвейс 3200, жемчужница 40 000, Indigo Cap 68 840.
- Редкость не зависит от порога — просто меньше спавнится.

# Эволюция дизайна (важно для origin_go)

Фиксированные узлы → игроки замапили их и-botили («логин, тот же спот, то же животное, каждые 15 минут») → годы просьб рандомизировать Q на спавн ([t=59696](https://www.havenandhearth.com/forum/viewtopic.php?t=59696)) → W16 добавил **дрейф шумовых полей** вместо статики. В обсуждении W16.1 (2025) звучит радикальная идея: «Raw resources should have no Quality until processed (seeds, rocks, ore, live animals, fish)» — сырое качество вообще убрать. Это готовый кейс «как не надо и как чинили» для вашей системы спотов.

Sources: [RoB: Foraging](https://ringofbrodgar.com/wiki/Foraging) · [RoB: Mining](https://ringofbrodgar.com/wiki/Mining) · [RoB: Fishing](https://ringofbrodgar.com/wiki/Fishing) · [RoB: Well](https://ringofbrodgar.com/wiki/Well) · [RoB: Tree](https://ringofbrodgar.com/wiki/Tree) · [RoB: Quality](https://ringofbrodgar.com/wiki/Quality) · [Как работают quality nodes](https://www.havenandhearth.com/forum/viewtopic.php?t=13760) · [Map generator (patch.py от loftar)](https://www.havenandhearth.com/forum/viewtopic.php?f=7&t=11222) · [Animal spawn mechanic changes](https://www.havenandhearth.com/forum/viewtopic.php?t=74397) · [Animals Quality](https://www.havenandhearth.com/forum/viewtopic.php?t=59696) · [Animal Wilderness Spawn](https://www.havenandhearth.com/forum/viewtopic.php?t=10431) · [Quality Nodes, overfarming](https://www.havenandhearth.com/forum/viewtopic.php?t=69940) · [Prelude: World 16](https://www.havenandhearth.com/forum/viewtopic.php?f=39&t=76519) · [Fandom: Finding high quality water, clay, and soil](https://havenandhearth.fandom.com/wiki/Finding_high_quality_water,_clay,_and_soil)