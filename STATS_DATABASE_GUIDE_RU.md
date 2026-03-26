# 📊 Руководство по просмотру статистики

Это руководство описывает все способы просмотра данных из базы данных статистики игры.

## 📑 Содержание

1. [Где хранится база данных](#-где-хранится-база-данных)
2. [Способ 1: Через Docker exec (Рекомендуемый)](#-способ-1-через-docker-exec-рекомендуемый)
3. [Способ 2: Одной командой](#-способ-2-одной-командой-без-входа-в-консоль)
4. [Способ 3: Скопировать БД на хост](#-способ-3-скопировать-бд-на-хост)
5. [Способ 4: Через API](#-способ-4-через-api-из-терминала)
6. [Продвинутые SQL запросы](#-продвинутые-sql-запросы)
7. [Полезные команды SQLite](#-полезные-команды-sqlite)
8. [Структура базы данных](#-структура-базы-данных)
9. [Частые проблемы](#-частые-проблемы)
10. [Советы и лучшие практики](#-советы-и-лучшие-практики)
11. [Дополнительные ресурсы](#-дополнительные-ресурсы)
12. [Быстрый справочник команд](#-быстрый-справочник-команд)

---

## 📍 Где хранится база данных

База данных SQLite хранится в Docker volume `shooter_stats_data`:

- **Внутри контейнера**: `/data/stats.db`
- **Docker volume**: `shooter_stats_data`
- **Путь к БД**: настраивается через `STATS_DB_PATH` (по умолчанию `/data/stats.db`)

---

## 🔍 Способ 1: Через Docker exec (Рекомендуемый)

Этот способ позволяет подключиться к базе данных напрямую внутри контейнера без копирования файлов.

### Шаг 1: Найти контейнер room-manager

```bash
docker ps | grep room-manager
```

Пример вывода:
```
CONTAINER ID   IMAGE                  COMMAND                  STATUS          PORTS                    NAMES
abc123456789   shooter-arena:latest  "/usr/local/bin/roo…"   Up 2 hours      0.0.0.0:8080->8080/tcp   hack2026mart-room-manager-1
```

Запомните имя контейнера (последняя колонка), например: `hack2026mart-room-manager-1`

### Шаг 2: Подключиться к интерактивной консоли SQLite

```bash
docker exec -it hack2026mart-room-manager-1 sqlite3 /data/stats.db
```

Если команда не найдена, установите sqlite3 в контейнер или используйте Способ 2.

### Шаг 3: Работа с базой данных

После подключения вы увидите приглашение `sqlite>`. Теперь доступны следующие команды:

#### Показать все таблицы

```sql
.tables
```

Ожидаемый вывод:
```
kills     players   rooms     sessions  shots     schema_version  map_stats
```

#### Показать схему таблицы

```sql
.schema players
.schema kills
.schema shots
.schema sessions
.schema map_stats
```

Или показать схему всех таблиц сразу:

```sql
.schema
```

#### Посмотреть данные из таблицы

```sql
-- Все игроки (первые 10)
SELECT * FROM players LIMIT 10;

-- Все убийства (последние 20)
SELECT * FROM kills ORDER BY timestamp DESC LIMIT 20;

-- Все сессии
SELECT * FROM sessions LIMIT 10;

-- Все выстрелы (последние 20)
SELECT * FROM shots ORDER BY timestamp DESC LIMIT 20;

-- Статистика по картам
SELECT * FROM map_stats;

-- Количество записей в каждой таблице
SELECT 'players' as table_name, COUNT(*) as count FROM players
UNION ALL SELECT 'kills', COUNT(*) FROM kills
UNION ALL SELECT 'shots', COUNT(*) FROM shots
UNION ALL SELECT 'sessions', COUNT(*) FROM sessions
UNION ALL SELECT 'map_stats', COUNT(*) FROM map_stats;
```

#### Полезные запросы

**Топ игроков по убийствам:**
```sql
SELECT 
    player_id,
    total_kills,
    total_deaths,
    ROUND(1.0 * total_kills / NULLIF(total_deaths, 0), 2) as kd_ratio,
    ROUND(1.0 * shots_hit / NULLIF(shots_fired, 0) * 100, 1) as accuracy_pct
FROM players 
ORDER BY total_kills DESC
LIMIT 10;
```

**Статистика по оружию:**
```sql
SELECT 
    CASE weapon_type 
        WHEN 1 THEN 'Pistol' 
        WHEN 2 THEN 'Rifle' 
        ELSE 'Unknown' 
    END as weapon_name,
    COUNT(*) as total_shots,
    SUM(CASE WHEN hit = 1 THEN 1 ELSE 0 END) as hits,
    ROUND(1.0 * SUM(CASE WHEN hit = 1 THEN 1 ELSE 0 END) / COUNT(*) * 100, 1) as accuracy_pct
FROM shots
GROUP BY weapon_type;
```

**Последние убийства с именами:**
```sql
SELECT 
    k.timestamp,
    k.killer_id,
    k.victim_id,
    CASE k.weapon_type 
        WHEN 1 THEN 'Pistol' 
        WHEN 2 THEN 'Rifle' 
    END as weapon
FROM kills k
ORDER BY k.timestamp DESC
LIMIT 20;
```

**Статистика по картам:**
```sql
SELECT 
    map_id,
    games_played,
    unique_players,
    ROUND(total_duration_seconds / 60.0, 1) as total_duration_min,
    last_played
FROM map_stats
ORDER BY games_played DESC;
```

**Активность по времени (сессии по дням):**
```sql
SELECT 
    DATE(start_time) as game_date,
    COUNT(*) as sessions_count,
    COUNT(DISTINCT player_id) as unique_players,
    ROUND(SUM(duration_seconds) / 60.0, 1) as total_playtime_min
FROM sessions
GROUP BY DATE(start_time)
ORDER BY game_date DESC;
```

**Детальная статистика игрока:**
```sql
SELECT 
    p.player_id,
    p.total_kills,
    p.total_deaths,
    ROUND(1.0 * p.total_kills / NULLIF(p.total_deaths, 0), 2) as kd_ratio,
    p.shots_fired,
    p.shots_hit,
    ROUND(1.0 * p.shots_hit / NULLIF(p.shots_fired, 0) * 100, 1) as accuracy_pct,
    p.pistol_kills,
    p.rifle_kills,
    ROUND(p.total_playtime_seconds / 60.0, 1) as playtime_min,
    p.first_seen,
    p.last_seen
FROM players p
WHERE p.player_id = '13134-988781';
```

**Топ игроков по точности (минимум 50 выстрелов):**
```sql
SELECT 
    player_id,
    shots_fired,
    shots_hit,
    ROUND(1.0 * shots_hit / shots_fired * 100, 1) as accuracy_pct,
    total_kills
FROM players
WHERE shots_fired >= 50
ORDER BY accuracy_pct DESC
LIMIT 10;
```

**Статистика убийств по оружию для конкретного игрока:**
```sql
SELECT 
    CASE weapon_type 
        WHEN 1 THEN 'Pistol' 
        WHEN 2 THEN 'Rifle' 
    END as weapon,
    COUNT(*) as kills
FROM kills
WHERE killer_id = '13134-988781'
GROUP BY weapon_type;
```

**Последние сессии с подробностями:**
```sql
SELECT 
    s.player_id,
    s.room_id,
    s.map_id,
    s.start_time,
    s.end_time,
    ROUND(s.duration_seconds / 60.0, 1) as duration_min,
    s.kills,
    s.deaths
FROM sessions s
ORDER BY s.start_time DESC
LIMIT 20;
```

**Самые популярные карты:**
```sql
SELECT 
    map_id,
    games_played,
    unique_players,
    ROUND(total_duration_seconds / 3600.0, 1) as total_hours,
    ROUND(1.0 * total_duration_seconds / games_played / 60.0, 1) as avg_game_min,
    last_played
FROM map_stats
ORDER BY games_played DESC;
```

**Статистика выстрелов по времени (по часам):**
```sql
SELECT 
    strftime('%Y-%m-%d %H:00', timestamp) as hour,
    COUNT(*) as total_shots,
    SUM(CASE WHEN hit = 1 THEN 1 ELSE 0 END) as hits,
    ROUND(1.0 * SUM(CASE WHEN hit = 1 THEN 1 ELSE 0 END) / COUNT(*) * 100, 1) as accuracy_pct
FROM shots
GROUP BY strftime('%Y-%m-%d %H:00', timestamp)
ORDER BY hour DESC
LIMIT 24;
```

**Топ дуэлей (кто кого чаще убивает):**
```sql
SELECT 
    killer_id,
    victim_id,
    COUNT(*) as kills_count,
    CASE weapon_type 
        WHEN 1 THEN 'Pistol' 
        WHEN 2 THEN 'Rifle' 
    END as favorite_weapon
FROM kills
GROUP BY killer_id, victim_id, weapon_type
HAVING COUNT(*) > 1
ORDER BY kills_count DESC
LIMIT 20;
```

**Статистика по комнатам:**
```sql
SELECT 
    r.room_id,
    r.map_id,
    r.created_at,
    r.stopped_at,
    r.total_players,
    COUNT(DISTINCT s.player_id) as unique_players_joined,
    COUNT(s.session_id) as total_sessions
FROM rooms r
LEFT JOIN sessions s ON r.room_id = s.room_id
GROUP BY r.room_id
ORDER BY r.created_at DESC
LIMIT 10;
```

#### Выйти из консоли

```sql
.quit
```

Или нажмите `Ctrl+D`.

---

## 🔍 Способ 2: Одной командой (без входа в консоль)

Можно выполнять запросы напрямую, без входа в интерактивный режим:

```bash
# Показать все таблицы
docker exec hack2026mart-room-manager-1 sqlite3 /data/stats.db ".tables"

# Показать схему всех таблиц
docker exec hack2026mart-room-manager-1 sqlite3 /data/stats.db ".schema"

# Показать схему конкретной таблицы
docker exec hack2026mart-room-manager-1 sqlite3 /data/stats.db ".schema players"

# Топ игроков
docker exec hack2026mart-room-manager-1 sqlite3 /data/stats.db \
  "SELECT player_id, total_kills, total_deaths FROM players ORDER BY total_kills DESC LIMIT 10;"

# Последние убийства
docker exec hack2026mart-room-manager-1 sqlite3 /data/stats.db \
  "SELECT killer_id, victim_id, timestamp FROM kills ORDER BY timestamp DESC LIMIT 10;"

# Количество записей в каждой таблице
docker exec hack2026mart-room-manager-1 sqlite3 /data/stats.db \
  "SELECT 'players' as tbl, COUNT(*) FROM players
   UNION ALL SELECT 'kills', COUNT(*) FROM kills
   UNION ALL SELECT 'shots', COUNT(*) FROM shots
   UNION ALL SELECT 'sessions', COUNT(*) FROM sessions;"

# Статистика конкретного игрока
docker exec hack2026mart-room-manager-1 sqlite3 /data/stats.db \
  "SELECT player_id, total_kills, total_deaths, shots_fired, shots_hit, 
   ROUND(1.0 * shots_hit / NULLIF(shots_fired, 0) * 100, 1) as accuracy 
   FROM players WHERE player_id = '13134-988781';"

# Экспорт в CSV (с заголовками)
docker exec hack2026mart-room-manager-1 sqlite3 /data/stats.db \
  -header -csv "SELECT * FROM players ORDER BY total_kills DESC LIMIT 10;" > players.csv

# Экспорт в JSON (требует jq)
docker exec hack2026mart-room-manager-1 sqlite3 /data/stats.db \
  -json "SELECT * FROM players LIMIT 10;"
```

---

## 🔍 Способ 3: Скопировать БД на хост

Если нужно работать с данными локально:

```bash
# Скопировать БД из контейнера
docker cp hack2026mart-room-manager-1:/data/stats.db ./stats.db

# Посмотреть локально
sqlite3 ./stats.db

# Вернуть обратно (если нужно)
docker cp ./stats.db hack2026mart-room-manager-1:/data/stats.db
```

---

## 🔍 Способ 4: Через API (из терминала)

Statistics API доступен на порту 8080:

```bash
# Статистика игрока
curl http://localhost:8080/stats/player/13134-988781 | jq

# Топ 10 игроков
curl http://localhost:8080/stats/players/top?limit=10 | jq

# Статистика оружия
curl http://localhost:8080/stats/weapons | jq

# Статистика карт
curl http://localhost:8080/stats/maps | jq

# Экспорт всех игроков в CSV
curl http://localhost:8080/stats/export/players --output players.csv

# Экспорт событий за период
curl "http://localhost:8080/stats/export/events?start_date=2024-01-01&end_date=2024-12-31" --output events.jsonl
```

Если `jq` не установлен, можно просто посмотреть JSON:

```bash
# Без форматирования
curl http://localhost:8080/stats/player/13134-988781

# С форматированием через python
curl http://localhost:8080/stats/player/13134-988781 | python -m json.tool
```

---

## � Продвинутые SQL запросы

### Анализ производительности игроков

**Игроки с лучшим K/D соотношением (минимум 10 убийств):**
```sql
SELECT 
    player_id,
    total_kills,
    total_deaths,
    ROUND(1.0 * total_kills / NULLIF(total_deaths, 0), 2) as kd_ratio,
    ROUND(1.0 * shots_hit / NULLIF(shots_fired, 0) * 100, 1) as accuracy_pct,
    ROUND(total_playtime_seconds / 3600.0, 1) as hours_played
FROM players
WHERE total_kills >= 10
ORDER BY kd_ratio DESC
LIMIT 10;
```

**Эффективность по времени (убийств в час):**
```sql
SELECT 
    player_id,
    total_kills,
    ROUND(total_playtime_seconds / 3600.0, 2) as hours_played,
    ROUND(1.0 * total_kills / NULLIF(total_playtime_seconds / 3600.0, 0), 1) as kills_per_hour
FROM players
WHERE total_playtime_seconds > 600
ORDER BY kills_per_hour DESC
LIMIT 10;
```

### Анализ оружия

**Сравнение эффективности оружия:**
```sql
SELECT 
    CASE weapon_type 
        WHEN 1 THEN 'Pistol' 
        WHEN 2 THEN 'Rifle' 
    END as weapon,
    COUNT(*) as total_shots,
    SUM(CASE WHEN hit = 1 THEN 1 ELSE 0 END) as hits,
    ROUND(1.0 * SUM(CASE WHEN hit = 1 THEN 1 ELSE 0 END) / COUNT(*) * 100, 1) as accuracy_pct,
    (SELECT COUNT(*) FROM kills k WHERE k.weapon_type = s.weapon_type) as total_kills,
    ROUND(1.0 * (SELECT COUNT(*) FROM kills k WHERE k.weapon_type = s.weapon_type) / 
          NULLIF(SUM(CASE WHEN hit = 1 THEN 1 ELSE 0 END), 0) * 100, 1) as kill_rate_pct
FROM shots s
GROUP BY weapon_type;
```

**Предпочтения игроков по оружию:**
```sql
SELECT 
    player_id,
    pistol_kills,
    rifle_kills,
    CASE 
        WHEN pistol_kills > rifle_kills THEN 'Pistol Player'
        WHEN rifle_kills > pistol_kills THEN 'Rifle Player'
        ELSE 'Balanced'
    END as player_type,
    total_kills
FROM players
WHERE total_kills > 0
ORDER BY total_kills DESC
LIMIT 20;
```

### Временной анализ

**Активность по дням недели:**
```sql
SELECT 
    CASE CAST(strftime('%w', start_time) AS INTEGER)
        WHEN 0 THEN 'Воскресенье'
        WHEN 1 THEN 'Понедельник'
        WHEN 2 THEN 'Вторник'
        WHEN 3 THEN 'Среда'
        WHEN 4 THEN 'Четверг'
        WHEN 5 THEN 'Пятница'
        WHEN 6 THEN 'Суббота'
    END as day_of_week,
    COUNT(*) as sessions,
    COUNT(DISTINCT player_id) as unique_players,
    ROUND(SUM(duration_seconds) / 3600.0, 1) as total_hours
FROM sessions
GROUP BY strftime('%w', start_time)
ORDER BY CAST(strftime('%w', start_time) AS INTEGER);
```

**Пиковые часы активности:**
```sql
SELECT 
    strftime('%H', start_time) as hour,
    COUNT(*) as sessions,
    COUNT(DISTINCT player_id) as unique_players,
    ROUND(AVG(duration_seconds) / 60.0, 1) as avg_session_min
FROM sessions
GROUP BY strftime('%H', start_time)
ORDER BY sessions DESC;
```

### Анализ сессий

**Средняя длительность сессий по картам:**
```sql
SELECT 
    map_id,
    COUNT(*) as sessions,
    ROUND(AVG(duration_seconds) / 60.0, 1) as avg_duration_min,
    ROUND(MIN(duration_seconds) / 60.0, 1) as min_duration_min,
    ROUND(MAX(duration_seconds) / 60.0, 1) as max_duration_min,
    ROUND(AVG(kills), 1) as avg_kills_per_session,
    ROUND(AVG(deaths), 1) as avg_deaths_per_session
FROM sessions
WHERE duration_seconds IS NOT NULL
GROUP BY map_id
ORDER BY sessions DESC;
```

**Самые длинные сессии:**
```sql
SELECT 
    player_id,
    map_id,
    start_time,
    ROUND(duration_seconds / 60.0, 1) as duration_min,
    kills,
    deaths,
    ROUND(1.0 * kills / NULLIF(deaths, 0), 2) as kd_ratio
FROM sessions
WHERE duration_seconds IS NOT NULL
ORDER BY duration_seconds DESC
LIMIT 10;
```

### Соревновательный анализ

**Топ соперников (кто с кем чаще сражается):**
```sql
SELECT 
    k1.killer_id as player1,
    k1.victim_id as player2,
    COUNT(*) as player1_kills,
    (SELECT COUNT(*) FROM kills k2 
     WHERE k2.killer_id = k1.victim_id 
     AND k2.victim_id = k1.killer_id) as player2_kills,
    COUNT(*) - (SELECT COUNT(*) FROM kills k2 
                WHERE k2.killer_id = k1.victim_id 
                AND k2.victim_id = k1.killer_id) as kill_difference
FROM kills k1
GROUP BY k1.killer_id, k1.victim_id
HAVING COUNT(*) > 2
ORDER BY COUNT(*) DESC
LIMIT 20;
```

**Статистика "мести" (кто кого убил после смерти):**
```sql
WITH kill_pairs AS (
    SELECT 
        k1.killer_id,
        k1.victim_id,
        k1.timestamp as kill_time,
        k2.timestamp as revenge_time
    FROM kills k1
    LEFT JOIN kills k2 ON k1.victim_id = k2.killer_id 
                       AND k1.killer_id = k2.victim_id
                       AND k2.timestamp > k1.timestamp
                       AND k2.timestamp < datetime(k1.timestamp, '+2 minutes')
)
SELECT 
    killer_id,
    victim_id,
    COUNT(*) as kills,
    SUM(CASE WHEN revenge_time IS NOT NULL THEN 1 ELSE 0 END) as revenges,
    ROUND(1.0 * SUM(CASE WHEN revenge_time IS NOT NULL THEN 1 ELSE 0 END) / COUNT(*) * 100, 1) as revenge_rate_pct
FROM kill_pairs
GROUP BY killer_id, victim_id
HAVING COUNT(*) > 3
ORDER BY kills DESC
LIMIT 20;
```

### Прогресс игроков

**Улучшение точности со временем:**
```sql
WITH player_progress AS (
    SELECT 
        player_id,
        DATE(timestamp) as game_date,
        COUNT(*) as shots,
        SUM(CASE WHEN hit = 1 THEN 1 ELSE 0 END) as hits,
        ROUND(1.0 * SUM(CASE WHEN hit = 1 THEN 1 ELSE 0 END) / COUNT(*) * 100, 1) as accuracy
    FROM shots
    GROUP BY player_id, DATE(timestamp)
)
SELECT 
    player_id,
    game_date,
    shots,
    accuracy,
    LAG(accuracy) OVER (PARTITION BY player_id ORDER BY game_date) as prev_accuracy,
    accuracy - LAG(accuracy) OVER (PARTITION BY player_id ORDER BY game_date) as accuracy_change
FROM player_progress
WHERE shots >= 20
ORDER BY player_id, game_date DESC;
```

### Экспорт данных

**Экспорт полной статистики игрока в CSV формат:**
```sql
.mode csv
.headers on
.output player_stats.csv

SELECT 
    p.player_id,
    p.total_kills,
    p.total_deaths,
    ROUND(1.0 * p.total_kills / NULLIF(p.total_deaths, 0), 2) as kd_ratio,
    p.shots_fired,
    p.shots_hit,
    ROUND(1.0 * p.shots_hit / NULLIF(p.shots_fired, 0) * 100, 1) as accuracy,
    p.pistol_kills,
    p.rifle_kills,
    ROUND(p.total_playtime_seconds / 3600.0, 2) as hours_played,
    p.first_seen,
    p.last_seen,
    (SELECT COUNT(DISTINCT map_id) FROM sessions WHERE player_id = p.player_id) as maps_played,
    (SELECT COUNT(*) FROM sessions WHERE player_id = p.player_id) as total_sessions
FROM players p
ORDER BY p.total_kills DESC;

.output stdout
```

**Экспорт истории убийств:**
```sql
.mode csv
.headers on
.output kill_history.csv

SELECT 
    k.timestamp,
    k.killer_id,
    k.victim_id,
    CASE k.weapon_type WHEN 1 THEN 'Pistol' WHEN 2 THEN 'Rifle' END as weapon,
    k.room_id
FROM kills k
ORDER BY k.timestamp DESC;

.output stdout
```

---

## 🛠️ Полезные команды SQLite

### Настройка отображения

```sql
-- Включить заголовки колонок
.headers on

-- Режим отображения (column, csv, json, list, table)
.mode column

-- Установить ширину колонок
.width 20 10 10 10

-- Красивая таблица
.mode table

-- JSON формат
.mode json
```

### Производительность

```sql
-- Показать план выполнения запроса
EXPLAIN QUERY PLAN 
SELECT * FROM players WHERE total_kills > 100;

-- Анализ использования индексов
PRAGMA index_list('players');
PRAGMA index_info('idx_players_kills');

-- Статистика базы данных
PRAGMA database_list;
PRAGMA table_info('players');

-- Размер базы данных
SELECT page_count * page_size as size_bytes 
FROM pragma_page_count(), pragma_page_size();
```

### Обслуживание базы данных

```sql
-- Проверка целостности
PRAGMA integrity_check;

-- Оптимизация (дефрагментация)
VACUUM;

-- Обновление статистики для оптимизатора
ANALYZE;

-- Проверка внешних ключей
PRAGMA foreign_key_check;
```

---

## 📋 Структура базы данных

### Таблица `players`

| Колонка | Тип | Описание |
|---------|-----|----------|
| player_id | TEXT | Уникальный ID игрока |
| first_seen | TIMESTAMP | Первое появление в игре |
| last_seen | TIMESTAMP | Последнее появление |
| total_kills | INTEGER | Всего убийств |
| total_deaths | INTEGER | Всего смертей |
| shots_fired | INTEGER | Всего выстрелов |
| shots_hit | INTEGER | Всего попаданий |
| pistol_kills | INTEGER | Убийств из пистолета |
| rifle_kills | INTEGER | Убийств из автомата |
| total_playtime_seconds | INTEGER | Общее время в игре |

### Таблица `kills`

| Колонка | Тип | Описание |
|---------|-----|----------|
| kill_id | INTEGER | Уникальный ID убийства |
| killer_id | TEXT | ID убийцы |
| victim_id | TEXT | ID жертвы |
| room_id | TEXT | ID комнаты |
| weapon_type | INTEGER | Тип оружия (1=пистолет, 2=автомат) |
| timestamp | TIMESTAMP | Время убийства |

### Таблица `shots`

| Колонка | Тип | Описание |
|---------|-----|----------|
| shot_id | INTEGER | Уникальный ID выстрела |
| player_id | TEXT | ID стрелка |
| room_id | TEXT | ID комнаты |
| weapon_type | INTEGER | Тип оружия |
| hit | BOOLEAN | Попал ли (1=да, 0=нет) |
| timestamp | TIMESTAMP | Время выстрела |

### Таблица `sessions`

| Колонка | Тип | Описание |
|---------|-----|----------|
| session_id | INTEGER | Уникальный ID сессии |
| player_id | TEXT | ID игрока |
| room_id | TEXT | ID комнаты |
| map_id | TEXT | ID карты |
| start_time | TIMESTAMP | Начало сессии |
| end_time | TIMESTAMP | Конец сессии |
| duration_seconds | INTEGER | Длительность в секундах |
| kills | INTEGER | Убийств за сессию |
| deaths | INTEGER | Смертей за сессию |

### Таблица `map_stats`

| Колонка | Тип | Описание |
|---------|-----|----------|
| map_id | TEXT | ID карты |
| games_played | INTEGER | Сколько раз играли |
| unique_players | INTEGER | Уникальных игроков |
| total_duration_seconds | INTEGER | Общая длительность |
| last_played | TIMESTAMP | Последняя игра |

---

## 🛠️ Частые проблемы

### Ошибка: `sqlite3: command not found`

SQLite3 может не быть в контейнере. Решения:

1. **Используйте Способ 3** (скопировать БД на хост)
2. **Установите sqlite3 в контейнер** (если есть apt/apk):
   ```bash
   docker exec hack2026mart-room-manager-1 apt-get update && \
   docker exec hack2026mart-room-manager-1 apt-get install -y sqlite3
   ```
3. **Используйте API** (Способ 4) - не требует sqlite3 в контейнере

### Ошибка: `unable to find container`

Проверьте имя контейнера:
```bash
docker ps -a | grep room-manager
```

Если контейнер остановлен, запустите его:
```bash
docker start hack2026mart-room-manager-1
```

Если имя контейнера другое, используйте правильное имя во всех командах.

### Ошибка: `no such table: players`

База данных пуста — игроки ещё не заходили в игру. Статистика создаётся после первых игровых событий (выстрелы, убийства, сессии).

Проверьте, что статистика включена:
```bash
docker exec hack2026mart-room-manager-1 env | grep STATS
```

Должно быть:
```
STATS_ENABLED=true
STATS_DB_PATH=/data/stats.db
```

### Ошибка: `database is locked`

SQLite не поддерживает одновременную запись. Если видите эту ошибку:

1. Закройте все открытые соединения к БД
2. Подождите несколько секунд
3. Попробуйте снова

Для чтения данных используйте read-only режим:
```bash
docker exec hack2026mart-room-manager-1 sqlite3 -readonly /data/stats.db "SELECT COUNT(*) FROM players;"
```

### Данные не обновляются

Проверьте логи room-manager:
```bash
docker logs hack2026mart-room-manager-1 | grep stats
```

Проверьте логи room контейнера:
```bash
docker logs <room-container-name> | grep stats
```

Убедитесь, что события отправляются:
```bash
# Проверить последние записи в таблице shots
docker exec hack2026mart-room-manager-1 sqlite3 /data/stats.db \
  "SELECT COUNT(*), MAX(timestamp) FROM shots;"
```

### База данных слишком большая

Очистить старые данные (ОСТОРОЖНО - удаляет данные!):

```sql
-- Удалить выстрелы старше 30 дней
DELETE FROM shots WHERE timestamp < datetime('now', '-30 days');

-- Удалить сессии старше 90 дней
DELETE FROM sessions WHERE start_time < datetime('now', '-90 days');

-- Удалить убийства старше 90 дней
DELETE FROM kills WHERE timestamp < datetime('now', '-90 days');

-- Оптимизировать базу после удаления
VACUUM;
```

Или создать архив и начать с чистой БД:
```bash
# Остановить room-manager
docker stop hack2026mart-room-manager-1

# Скопировать БД
docker cp hack2026mart-room-manager-1:/data/stats.db ./stats_backup_$(date +%Y%m%d).db

# Удалить старую БД
docker exec hack2026mart-room-manager-1 rm /data/stats.db

# Запустить room-manager (создаст новую БД)
docker start hack2026mart-room-manager-1
```

---

## 💡 Советы и лучшие практики

### Регулярное резервное копирование

Создайте скрипт для автоматического бэкапа:

```bash
#!/bin/bash
# backup_stats.sh

BACKUP_DIR="./stats_backups"
DATE=$(date +%Y%m%d_%H%M%S)
CONTAINER="hack2026mart-room-manager-1"

mkdir -p "$BACKUP_DIR"
docker cp "$CONTAINER:/data/stats.db" "$BACKUP_DIR/stats_$DATE.db"

# Удалить бэкапы старше 30 дней
find "$BACKUP_DIR" -name "stats_*.db" -mtime +30 -delete

echo "Backup created: $BACKUP_DIR/stats_$DATE.db"
```

Запускайте через cron (каждый день в 3:00):
```bash
0 3 * * * /path/to/backup_stats.sh
```

### Мониторинг размера базы данных

```bash
# Проверить размер БД
docker exec hack2026mart-room-manager-1 du -h /data/stats.db

# Детальная статистика по таблицам
docker exec hack2026mart-room-manager-1 sqlite3 /data/stats.db \
  "SELECT name, 
          (SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND tbl_name=m.name) as indexes,
          (SELECT COUNT(*) FROM pragma_table_info(m.name)) as columns
   FROM sqlite_master m WHERE type='table' AND name NOT LIKE 'sqlite_%';"
```

### Оптимизация производительности

Периодически запускайте оптимизацию:

```bash
docker exec hack2026mart-room-manager-1 sqlite3 /data/stats.db "VACUUM; ANALYZE;"
```

### Экспорт для анализа в Excel/Google Sheets

```bash
# Экспорт топ игроков в CSV
docker exec hack2026mart-room-manager-1 sqlite3 -header -csv /data/stats.db \
  "SELECT player_id, total_kills, total_deaths, 
          ROUND(1.0 * total_kills / NULLIF(total_deaths, 0), 2) as kd_ratio,
          ROUND(1.0 * shots_hit / NULLIF(shots_fired, 0) * 100, 1) as accuracy
   FROM players ORDER BY total_kills DESC LIMIT 100;" > top_players.csv
```

Откройте `top_players.csv` в Excel или Google Sheets для визуализации.

### Создание пользовательских представлений (Views)

Для часто используемых запросов создайте представления:

```sql
-- Представление для топ игроков
CREATE VIEW IF NOT EXISTS top_players AS
SELECT 
    player_id,
    total_kills,
    total_deaths,
    ROUND(1.0 * total_kills / NULLIF(total_deaths, 0), 2) as kd_ratio,
    ROUND(1.0 * shots_hit / NULLIF(shots_fired, 0) * 100, 1) as accuracy,
    ROUND(total_playtime_seconds / 3600.0, 1) as hours_played
FROM players
WHERE total_kills > 0
ORDER BY total_kills DESC;

-- Использование
SELECT * FROM top_players LIMIT 10;
```

### Интеграция с внешними инструментами

**Grafana + SQLite:**
- Установите Grafana SQLite datasource plugin
- Подключите stats.db
- Создайте дашборды с графиками

**Python анализ:**
```python
import sqlite3
import pandas as pd

conn = sqlite3.connect('stats.db')
df = pd.read_sql_query("SELECT * FROM players", conn)
print(df.describe())
conn.close()
```

**Jupyter Notebook:**
```python
%load_ext sql
%sql sqlite:///stats.db
%sql SELECT * FROM players LIMIT 5
```

---

## 🛠️ Частые проблемы

### Ошибка: `sqlite3: command not found`

SQLite3 может не быть в контейнере. Решения:

1. **Используйте Способ 3** (скопировать БД на хост)
2. **Установите sqlite3 в контейнер** (если есть apt/apk):
   ```bash
   docker exec hack2026mart-room-manager-1 apt-get update && \
   docker exec hack2026mart-room-manager-1 apt-get install -y sqlite3
   ```

### Ошибка: `unable to find container`

Проверьте имя контейнера:
```bash
docker ps -a | grep room-manager
```

Если контейнер остановлен, запустите его:
```bash
docker start hack2026mart-room-manager-1
```

### Ошибка: `no such table: players`

База данных пуста — игроки ещё не заходили в игру. Статистика создаётся после первых игровых событий (выстрелы, убийства, сессии).

## 📖 Дополнительные ресурсы

### Документация

- [SQLite официальная документация](https://www.sqlite.org/docs.html)
- [SQLite SQL синтаксис](https://www.sqlite.org/lang.html)
- [SQLite команды CLI](https://www.sqlite.org/cli.html)
- [SQLite функции и операторы](https://www.sqlite.org/lang_corefunc.html)
- [Docker exec справка](https://docs.docker.com/engine/reference/commandline/exec/)

### Полезные SQL функции

**Работа с датами:**
```sql
-- Текущее время
SELECT datetime('now');

-- Форматирование даты
SELECT strftime('%Y-%m-%d %H:%M:%S', timestamp) FROM kills LIMIT 1;

-- Разница во времени
SELECT julianday('now') - julianday(last_seen) as days_ago FROM players;
```

**Агрегатные функции:**
```sql
-- Среднее, минимум, максимум
SELECT AVG(total_kills), MIN(total_kills), MAX(total_kills) FROM players;

-- Медиана (через percentile)
SELECT total_kills FROM players ORDER BY total_kills LIMIT 1 
OFFSET (SELECT COUNT(*) FROM players) / 2;

-- Стандартное отклонение (требует расширение)
SELECT AVG(total_kills) as mean,
       SQRT(AVG(total_kills * total_kills) - AVG(total_kills) * AVG(total_kills)) as stddev
FROM players;
```

**Условная логика:**
```sql
-- CASE выражения
SELECT player_id,
       total_kills,
       CASE 
           WHEN total_kills >= 100 THEN 'Elite'
           WHEN total_kills >= 50 THEN 'Veteran'
           WHEN total_kills >= 10 THEN 'Regular'
           ELSE 'Beginner'
       END as rank
FROM players;
```

### Примеры интеграции

**Bash скрипт для ежедневного отчета:**
```bash
#!/bin/bash
# daily_report.sh

CONTAINER="hack2026mart-room-manager-1"
DATE=$(date +%Y-%m-%d)

echo "=== Daily Statistics Report for $DATE ==="
echo ""

echo "Total Players:"
docker exec $CONTAINER sqlite3 /data/stats.db \
  "SELECT COUNT(*) FROM players;"

echo ""
echo "Games Played Today:"
docker exec $CONTAINER sqlite3 /data/stats.db \
  "SELECT COUNT(*) FROM sessions WHERE DATE(start_time) = '$DATE';"

echo ""
echo "Top 5 Players Today:"
docker exec $CONTAINER sqlite3 -header -column /data/stats.db \
  "SELECT player_id, SUM(kills) as kills 
   FROM sessions 
   WHERE DATE(start_time) = '$DATE' 
   GROUP BY player_id 
   ORDER BY kills DESC 
   LIMIT 5;"
```

**PowerShell скрипт для Windows:**
```powershell
# daily_report.ps1

$container = "hack2026mart-room-manager-1"
$date = Get-Date -Format "yyyy-MM-dd"

Write-Host "=== Daily Statistics Report for $date ===" -ForegroundColor Green

Write-Host "`nTotal Players:"
docker exec $container sqlite3 /data/stats.db "SELECT COUNT(*) FROM players;"

Write-Host "`nGames Played Today:"
docker exec $container sqlite3 /data/stats.db `
  "SELECT COUNT(*) FROM sessions WHERE DATE(start_time) = '$date';"

Write-Host "`nTop 5 Players Today:"
docker exec $container sqlite3 -header -column /data/stats.db `
  "SELECT player_id, SUM(kills) as kills 
   FROM sessions 
   WHERE DATE(start_time) = '$date' 
   GROUP BY player_id 
   ORDER BY kills DESC 
   LIMIT 5;"
```

### Визуализация данных

**Создание графиков с gnuplot:**
```bash
# Экспорт данных для графика
docker exec hack2026mart-room-manager-1 sqlite3 -csv /data/stats.db \
  "SELECT DATE(start_time) as date, COUNT(*) as sessions 
   FROM sessions 
   GROUP BY DATE(start_time) 
   ORDER BY date;" > sessions_per_day.csv

# Создать график (требует gnuplot)
gnuplot <<EOF
set datafile separator ","
set xdata time
set timefmt "%Y-%m-%d"
set format x "%m/%d"
set xlabel "Date"
set ylabel "Sessions"
set title "Sessions per Day"
set terminal png size 800,600
set output "sessions_graph.png"
plot "sessions_per_day.csv" using 1:2 with lines title "Sessions"
EOF
```

### Автоматизация с cron

Добавьте в crontab для автоматических задач:

```bash
# Редактировать crontab
crontab -e

# Добавить задачи:
# Бэкап каждый день в 3:00
0 3 * * * /path/to/backup_stats.sh

# Оптимизация каждое воскресенье в 4:00
0 4 * * 0 docker exec hack2026mart-room-manager-1 sqlite3 /data/stats.db "VACUUM; ANALYZE;"

# Ежедневный отчет в 23:00
0 23 * * * /path/to/daily_report.sh | mail -s "Daily Stats Report" admin@example.com
```

---

## 🎯 Быстрый справочник команд

### Подключение к БД
```bash
docker exec -it hack2026mart-room-manager-1 sqlite3 /data/stats.db
```

### Основные команды SQLite
```sql
.tables              -- Показать все таблицы
.schema              -- Показать схему всех таблиц
.schema players      -- Показать схему таблицы players
.headers on          -- Включить заголовки
.mode column         -- Режим колонок
.mode csv            -- Режим CSV
.quit                -- Выход
```

### Топ запросы
```sql
-- Топ игроков
SELECT player_id, total_kills, total_deaths FROM players ORDER BY total_kills DESC LIMIT 10;

-- Статистика оружия
SELECT weapon_type, COUNT(*) as shots, SUM(hit) as hits FROM shots GROUP BY weapon_type;

-- Последние убийства
SELECT * FROM kills ORDER BY timestamp DESC LIMIT 10;

-- Активные игроки сегодня
SELECT COUNT(DISTINCT player_id) FROM sessions WHERE DATE(start_time) = DATE('now');
```

### Экспорт данных
```bash
# CSV экспорт
docker exec hack2026mart-room-manager-1 sqlite3 -header -csv /data/stats.db \
  "SELECT * FROM players;" > players.csv

# JSON экспорт (если поддерживается)
docker exec hack2026mart-room-manager-1 sqlite3 -json /data/stats.db \
  "SELECT * FROM players LIMIT 10;"
```

### Обслуживание
```bash
# Проверка целостности
docker exec hack2026mart-room-manager-1 sqlite3 /data/stats.db "PRAGMA integrity_check;"

# Оптимизация
docker exec hack2026mart-room-manager-1 sqlite3 /data/stats.db "VACUUM; ANALYZE;"

# Размер БД
docker exec hack2026mart-room-manager-1 du -h /data/stats.db
```

---

**Приятной игры! 🎮🔴**
