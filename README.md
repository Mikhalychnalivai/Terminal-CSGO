# Решение кейса SSH Arena (shooter)

Минимальная многопользовательская SSH-игра на Go:
- `cmd/gateway` — точка входа SSH и терминальный рендерер
- `cmd/room` — сервер комнаты реального времени для одной комнаты (один контейнер = одна комната)
- `cmd/room-manager` — создаёт/запускает контейнеры комнат динамически через Docker API
- `internal/game/wad` — загрузчик WAD (карта, `PLAYPAL`, патчи стен, полы)
- Графика: **стены** используют блочные глифы (`█▓▒░…`) + **`PLAYPAL` RGB**; **потолок/пол** используют классическую градацию яркости (`@#8&…`). HUD-пистолет: WAD **`PISGA0`…`PISGD0`**. **Другие игроки**: встроенный **морпех в стиле shooter** (шлем/визор, зелёная броня, винтовка, ботинки) с **8 ракурсами биллборда**. Рекомендуется терминал с поддержкой Truecolor.

## Требования

- Docker + Docker Compose
- `SHOOTER.WAD` в `shooter wed/SHOOTER.WAD`

## Запуск

```bash
docker compose up --build
```

Подключитесь из другого терминала:

```bash
ssh -p 2222 any@localhost
```

## Управление

- `W` / `S` вперёд / назад
- `A` / `D` поворот
- `Space` огонь (трассер + анимация пистолета по времени, ~17 кадров/с на серию выстрела)
- `q` выход

Gateway читает `WAD_PATH` (по умолчанию `/assets/SHOOTER.WAD` в Docker) для настоящих текстур.

## Сборка Docker (TLS / модули)

Если сборка падает с `sum.golang.org` / `tls: received record with version…`, в **Dockerfile** уже задано `GOSUMDB=off` (без проверки checksum DB). Если не качаются модули с `proxy.golang.org`, соберите с другим прокси:

```bash
docker compose build --build-arg GOPROXY=https://goproxy.io,direct
```

Локально имеет смысл выполнить `go mod tidy` и закоммитить **`go.sum`** — тогда версии зафиксированы.

### Docker Hub: `127.0.0.1:12334` / `actively refused`

Если сборка падает на `FROM alpine:3.20` или `golang:...` с текстом вроде `connecting to 127.0.0.1:12334`, Docker ходит в интернет **через прокси**, а на этом порту никто не слушает.

1. **Docker Desktop** → *Settings* → *Resources* → *Proxies* — отключите или поправьте адрес.
2. В PowerShell проверьте: `Get-ChildItem Env:*proxy*`. Уберите `HTTP_PROXY` / `HTTPS_PROXY` / `ALL_PROXY` из переменных пользователя/системы, если они указывают на `127.0.0.1:12334`.
3. Иногда прокси задаётся не Docker Desktop, а системными настройками Windows (реестр `HKCU`). Проверьте:

```powershell
Get-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings' `
  -Name ProxyEnable,ProxyServer -ErrorAction SilentlyContinue |
  Select-Object ProxyEnable,ProxyServer
```

Если `ProxyEnable = 1` и `ProxyServer = http://127.0.0.1:12334`, отключите прокси в Windows (*Network & Internet* -> *Proxy*) или исправьте/запустите приложение, которое поднимает этот порт.
4. Либо запустите сборку без прокси **в этой сессии PowerShell** (если прокси задан в переменных окружения, а не только в системных настройках Windows):

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\docker-up-no-proxy.ps1
```

## Примечания

- **Низкая задержка:** тик комнаты по умолчанию **16 мс** (~62 Гц), переменная **`ROOM_TICK_MS`** (8–100) в `room-manager` / env контейнера `room`. **TCP_NODELAY** на связке gateway↔room. **Стены/оружие** в JSON шлются только в **первом** полном `state` на игрока; дальше только позиции — меньше трафика и быстрее отклик по LAN/Wi‑Fi.
- Если в комнате **один** игрок, в мир добавляется демо-**MARINE** (`__demo_marine__`) впереди по взгляду — чтобы смотреть спрайт без второго клиента.
- Игрок может создать комнату или присоединиться к существующей по ID комнаты.
- При создании комнаты игрок автоматически присоединяется.
- Каждая комната — это изолированный контейнер (`room-<room_id>`), управляемый `room-manager`.
- Общая карта загружается из `SHOOTER.WAD` (по умолчанию `E1M2`).
- Точки появления (где появляется игрок): по умолчанию включён `SPAWN_MODE=scatter`, он раскидывает стартовые точки по проходимой зоне карты симметрично. Параметры: `SPAWN_COUNT`, `SPAWN_MIN_DIST`, `SPAWN_SYMMETRY` (например, `4` для зеркальной симметрии).
- Точки оружия извлекаются из lump THINGS и отображаются как `W`.
