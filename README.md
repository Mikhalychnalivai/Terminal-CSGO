# Решение кейса SSH Arena AEZA

Минимальная многопользовательская SSH-игра на Go:
- `cmd/gateway` - выступает SSH-точкой входа и терминальным рендерером, 
- `cmd/room` - реализует realtime-сервер для одной игровой комнаты (один контейнер = одна комната), 
- `cmd/room-manager` - динамически создаёт и запускает контейнеры комнат через Docker API, 
- `internal/game/wad` - отвечает за загрузку WAD-файлов (карта, палитра PLAYPAL, патчи стен, плоскости).
- Графическая часть реализована следующим образом: стены используют блочные символы (█▓▒░…) с RGB-цветами из PLAYPAL, потолок и пол отрисовываются через классическую рампу яркости (@#8&…), HUD с пистолетом использует WAD-спрайты PISGA0…PISGD0, другие игроки отображаются как встроенные морпехи в стиле Doom (шлем/визор, зелёная броня, винтовка, ботинки) с 8 ракурсами billboard, рекомендуется использование truecolor-терминала для полноценной цветопередачи.

## Requirements

- Docker + Docker Compose
- `DOOM.WAD` at `doom wed/DOOM.WAD`
- Карты-редактор: `doom wed/map1.json`, `doom wed/map2.json` (копируются в образ как `/assets/maps/map1.json`, `map2.json`)

## Карты JSON и выбор при создании комнаты

При **создании комнаты** через SSH шлюз после ввода `Room ID` спрашивается карта:

- **`1`** или **`map1`** — загрузка `map1.json`
- **`2`** или **`map2`** — загрузка `map2.json`
- **`wad`** или **`doom`** — классическая карта из `DOOM.WAD` (как раньше, см. `WAD_MAP` у `room-manager`)

Через API `room-manager`:

```json
POST /rooms/create
{"room_id": "arena", "map_id": "2"}
```

Если `map_id` не указан, по умолчанию берётся **`1`** (переменная окружения `DEFAULT_MAP_ID` у `room-manager`).

Для JSON-карт спавны **по умолчанию из `Start` / `End` в файле**. Общий **`SPAWN_MODE=scatter`** в compose (для карт **WAD**) **на JSON не влияет**. Разброс по карте включается только явно у **`room-manager`**: **`JSON_USE_SCATTER=1`** или **`JSON_SPAWN_MODE=scatter`** (оба пробрасывают в контейнер `room` флаг **`JSON_USE_SCATTER=1`**).

**Если «ничего не менялось»:** чаще всего не пересобран образ (`docker compose up -d --build`), контейнер `room-*` остался старым, или при входе выбран не тот режим карты (нужно **создать комнату** и указать **`1`/`2`**, не `wad`). В логах контейнера `room` после пересборки должна быть строка вида `json map loaded ... title=map1 ...` или `room starting: JSON_MAP_PATH=...`.

Формат JSON по смыслу близок к **Wolfenstein 3D (1993)**: сетка клеток, **стена между двумя клетками** задаётся гранями соседей (или цельный блок — все четыре грани «закрыты»). Движок строит коллизии так: цельные кубы стен заливаются целиком, **внутренние** вертикальные/горизонтальные грани — **одной** линией клеток (без дыр и без лишнего дублирования).

Опции (прокинь в `room-manager`, они передаются в контейнер `room` для JSON-карт):

- **`JSON_MAP_SCALE`** — сколько клеток движка на одну клетку редактора (по умолчанию **2**, минимум 2).
- **`JSON_MAP_FLIP_Y`** — `1`, если карта вверх ногами / спавн не там: переворачивает ось Y как в классической сетке «математика vs экран».

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
- `Space` fire (tracer + pistol anim по времени, ~17 кадров/с на серию выстрела)
- `q` quit

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

- **Низкая задержка:** тик комнаты по умолчанию **16 ms** (~62 Гц), переменная **`ROOM_TICK_MS`** (8–100) в `room-manager` / env контейнера `room`. **TCP_NODELAY** на связке gateway↔room. **Стены/оружие** в JSON шлются только в **первом** полном `state` на игрока; дальше только позиции — меньше трафика и быстрее отклик по LAN/Wi‑Fi.
- Если в комнате **один** игрок, в мир добавляется демо-**MARINE** (`__demo_marine__`) впереди по взгляду — чтобы смотреть спрайт без второго клиента.
- Player can create room or join existing room by room ID.
- On room creation, player auto-joins.
- Each room is an isolated room container (`room-<room_id>`) managed by `room-manager`.
- Shared map is loaded from `DOOM.WAD` (`E1M2` by default).
- Spawns (где появляется игрок): по умолчанию включён `SPAWN_MODE=scatter`, он раскидывает стартовые точки по проходимой зоне карты симметрично. Параметры: `SPAWN_COUNT`, `SPAWN_MIN_DIST`, `SPAWN_SYMMETRY` (например, `4` для зеркальной симметрии).
- Weapon points are extracted from THINGS lump and rendered as `W`.
