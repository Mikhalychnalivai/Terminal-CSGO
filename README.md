# DOOM SSH Arena (MVP)

Minimal multiplayer SSH game in Go:
- `cmd/gateway` - SSH entrypoint and terminal renderer
- `cmd/room` - realtime room server for one room (one container = one room)
- `cmd/room-manager` - creates/starts room containers dynamically via Docker API
- `internal/game/wad` - WAD loader (map, `PLAYPAL`, wall patches, flats)
- Graphics: **walls** use block glyphs (`█▓▒░…`) + **`PLAYPAL` RGB**; **ceiling/floor** use the classic luma ramp (`@#8&…`). HUD: WAD pistol **`PISGA0`…`PISGD0`** или встроенный fallback; отдельный **ASCII-автомат** (справа от центра). **Other players**: built-in **Doom-style marine** (зоны: шлем / броня / оружие / ноги по позиции символа) с **8 billboard angles**. Truecolor terminal recommended.

## Стек (зависимости и компоненты)

**Go (прямые модули)** — в `go.mod` по-прежнему одна зависимость:

- [`github.com/gliderlabs/ssh`](https://github.com/gliderlabs/ssh) — SSH-сервер в `cmd/gateway`.

**Транзитивно** (подтягиваются сами, см. `go.sum`): `github.com/anmitsu/go-shlex`, `golang.org/x/crypto`, `golang.org/x/sys`, `golang.org/x/term`. **Новых пакетов в `require` мы не добавляли** — редактор карт, room-manager API и прочее на стандартной библиотеке и `internal/…`.

**Внутренние пакеты / бинарники (не отдельные модули):**

- `internal/mapedit` — интерактивный редактор сетки (SSH и `cmd/mapbuilder`).
- `internal/game/jsonmap` — загрузка JSON-карт, `SpawnCells`, масштаб `JSON_MAP_SCALE` (в т.ч. **1** для карт из редактора).
- `cmd/mapbuilder` — CLI-редактор карты без SSH.
- Docker: том `doom_ssh_room_maps` для пользовательских JSON при `map_id=custom` (см. `docker-compose.yml`).

## Requirements

- Docker + Docker Compose
- `DOOM.WAD` at `map/DOOM.WAD`
- Карта по умолчанию в образе: `map/corridor5.json` → `/assets/maps/corridor5.json`. Свои JSON можно добавить в образ (Dockerfile `COPY`) или смонтировать в контейнер `room` и указать `JSON_MAP_PATH`.

## Карты JSON и выбор при создании комнаты

При **создании комнаты** через SSH шлюз после ввода `Room ID` спрашивается карта:

- **`1`** (Enter) — **`corridor5`** (коридор из пяти комнат), то же что `map_id` **`corridor5`** / **`default`** у `room-manager`.
- **`2`** — ввести свой **`map_id`**, который понимает `room-manager` (сейчас поддерживаются **`1`**, **`corridor5`**, **`corridor`**, **`default`**, **`map`** — все ведут на `corridor5.json`). Другие имена без файла в образе дадут ошибку при старте `room`.
- **`3`** — встроенный редактор карты в SSH; карта уходит в `room-manager` как **`custom`** с телом **`map_json`** (нужен том `room_maps` в compose, см. «Стек»).

Через API `room-manager`:

```json
POST /rooms/create
{"room_id": "arena", "map_id": "corridor5"}
```

Если `map_id` не указан, по умолчанию берётся **`corridor5`** (переменная окружения `DEFAULT_MAP_ID` у `room-manager`).

Для JSON-карт спавны **по умолчанию из `Start` / `End` в файле**. Общий **`SPAWN_MODE=scatter`** в compose (для карт **WAD**) **на JSON не влияет**. Разброс по карте включается только явно у **`room-manager`**: **`JSON_USE_SCATTER=1`** или **`JSON_SPAWN_MODE=scatter`** (оба пробрасывают в контейнер `room` флаг **`JSON_USE_SCATTER=1`**).

**Если «ничего не менялось»:** чаще всего не пересобран образ (`docker compose up -d --build`), контейнер `room-*` остался старым. В логах `room` после пересборки должна быть строка вида `json map loaded ... title=corridor5 ...` или `room starting: JSON_MAP_PATH=...`.

Формат JSON по смыслу близок к **Wolfenstein 3D (1993)**: сетка клеток, **стена между двумя клетками** задаётся гранями соседей (или цельный блок — все четыре грани «закрыты»). Движок строит коллизии так: цельные кубы стен заливаются целиком, **внутренние** вертикальные/горизонтальные грани — **одной** линией клеток (без дыр и без лишнего дублирования).

Опции (прокинь в `room-manager`, они передаются в контейнер `room` для JSON-карт):

- **`JSON_MAP_SCALE`** — сколько клеток движка на одну клетку редактора (по умолчанию **2**, минимум **1**). Для карт из пункта меню **[3]** `room-manager` сам выставляет **`JSON_MAP_SCALE=1`**, чтобы сетка плана совпадала с коллизией.
- **`JSON_MAP_FLIP_Y`** — `1`, если карта вверх ногами / спавн не там: переворачивает ось Y как в классической сетке «математика vs экран».

## Производительность и трафик

- **`ROOM_TICK_MS`** (контейнер `room`) — период рассылки `state`, по умолчанию **4** ms (~250 Гц). Больше (например **16–33**) — меньше JSON/сек и нагрузка.
- **`GATEWAY_RENDER_MS`** (контейнер `gateway`) — интервал отрисовки кадра в SSH, по умолчанию **8** ms (~125 FPS). Снапшоты с room читаются сразу; лимит только на **вывод кадра** в терминал.
- Компактный `state` без `walls`/`weapons` после первого полного кадра; на клиенте кэш (как раньше).
- На **room**: один `json.Marshal` на тик на тип сообщения, канал с **coalescing** при отставании клиента.

Если нужно снизить нагрузку на слабом ПК: `GATEWAY_RENDER_MS=16` или `33`, `ROOM_TICK_MS=16` или `25`.

## Run

```bash
docker compose up --build
```

Connect from another terminal:

```bash
ssh -p 2222 any@localhost
```

## Controls

- `W` / `S` forward / back
- `A` / `D` turn
- `Space` fire (анимация оружия; урон: пистолет **20**, автомат **33**; без патронов в текущей обойме/магазине — нет выстрела и нет анимации огня)
- `1` / `2` переключение HUD: **автомат** (ASCII) / **пистолет** (WAD `PISG*` или fallback)
- Слева снизу: **HP** (красным) и **ARM** (синим); спавн **100 HP**, **0 брони**; сначала снимается броня, потом HP
- После смерти: **`r`** возродиться; **`q`** — выход (и из меню смерти)

Gateway reads `WAD_PATH` (default `/assets/DOOM.WAD` in Docker) for real textures.

## Docker build (TLS / модули)

Если сборка падает с `sum.golang.org` / `tls: received record with version…`, в **Dockerfile** уже задано `GOSUMDB=off` (без проверки checksum DB). Если не качаются модули с `proxy.golang.org`, собери с другим прокси:

```bash
docker compose build --build-arg GOPROXY=https://goproxy.io,direct
```

Локально имеет смысл выполнить `go mod tidy` и закоммитить **`go.sum`** — тогда версии зафиксированы.

### Docker Hub: `127.0.0.1:12334` / `actively refused`

Если сборка падает на `FROM alpine:3.20` или `golang:...` с текстом вроде `connecting to 127.0.0.1:12334`, Docker ходит в интернет **через прокси**, а на этом порту никто не слушает.

1. **Docker Desktop** → *Settings* → *Resources* → *Proxies* — отключи или поправь адрес.
2. В PowerShell проверь: `Get-ChildItem Env:*proxy*`. Убери `HTTP_PROXY` / `HTTPS_PROXY` / `ALL_PROXY` из переменных пользователя/системы, если они указывают на `127.0.0.1:12334`.
3. Иногда прокси задается не Docker Desktop, а системными настройками Windows (реестр `HKCU`). Проверь:

```powershell
Get-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings' `
  -Name ProxyEnable,ProxyServer -ErrorAction SilentlyContinue |
  Select-Object ProxyEnable,ProxyServer
```

Если `ProxyEnable = 1` и `ProxyServer = http://127.0.0.1:12334`, отключи прокси в Windows (*Network & Internet* -> *Proxy*) или исправь/запусти приложение, которое поднимает этот порт.
4. Либо запусти сборку без прокси **в этой сессии PowerShell** (если прокси задан в переменных окружения, а не только в системных настройках Windows):

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\docker-up-no-proxy.ps1
```

## Notes

- **Визуал мира (клиент):** мир рисуется **3D-мешем** (пол/потолок/стены из треугольников + z-buffer), в духе [Asciipocalypse](https://github.com/wonrzrzeczny/Asciipocalypse) — рампа символов по глубине (fog + дизеринг по пикселю). Это **не** Unity/Unreal **NavMesh** (полигоны для ИИ/пути). Карта приходит с сервера как **сетка стен**; меш строится из неё для картинки (**контуры**, **сетка** на полу/потолке, условный **«кирпич»**, разный оттенок стен по осям). Глубина по колонкам для спрайтов других игроков берётся из того же прохода меша.
- **Движение и «не клеточка»:** на сервере и в gateway-предикции — пакет `internal/game/nav` (**круг** + **slide** вдоль стен).
- **Классический NavMesh (2D):** пакет `internal/game/navmesh` — при загрузке комнаты из сетки `blocked` строятся **ось-ориентированные прямоугольники** (покрытие проходимых клеток), соседи = полигоны с общей стороной, **A-star** по графу полигонов. API: **`(*Mesh).FindPath(sx,sy,ex,ey)`**, на комнате — **`(*Room).FindNavPath(...)`** (для ботов, «идти к точке», отладки). **WASD** по-прежнему через **nav** (коллизия), не через «прилипание к пути» — при необходимости можно связать сами.
- **Низкая задержка:** тик комнаты по умолчанию **4 ms** (~250 Гц), переменная **`ROOM_TICK_MS`** (4–100) в `room-manager` / env контейнера `room`. **TCP_NODELAY** на связке gateway↔room. **Стены/оружие** в JSON шлются только в **первом** полном `state` на игрока; дальше только позиции — меньше трафика и быстрее отклик по LAN/Wi‑Fi. На клиенте gateway — **prediction** (локальный шаг как на сервере) и **сглаживание** к авторитетному state.
- **Боезапас:** пистолет (**1**) — **10** в обойме, запас **бесконечный** (**R** добивает обойму до 10, анимация ~0,75 с). Автомат (**2**) — **30** + **60** в запасе **только при первом входе в комнату**; после **смерти и респавна** оба слота — как в момент смерти. **R** у живого: перезарядка выбранного оружия (автомат — из запаса, пока магазин &lt; 30; анимация автомата ~1 с). У мёртвого **R** — респавн.
- Player can create room or join existing room by room ID.
- On room creation, player auto-joins.
- Each room is an isolated room container (`room-<room_id>`) managed by `room-manager`.
- Карта по умолчанию в Docker — JSON **corridor5**; при запуске `room` с `WAD_PATH` + `WAD_MAP` загружается уровень из `DOOM.WAD`.
- Spawns (где появляется игрок): по умолчанию включён `SPAWN_MODE=scatter`, он раскидывает стартовые точки по проходимой зоне карты симметрично. Параметры: `SPAWN_COUNT`, `SPAWN_MIN_DIST`, `SPAWN_SYMMETRY` (например, `4` для зеркальной симметрии).
- Weapon points are extracted from THINGS lump and rendered as `W`.
