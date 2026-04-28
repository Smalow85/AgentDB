Архитектура СУБД
Структура пакетов
text
sql-db/
├── cmd/
│   └── sqldb/
│       └── main.go                 # Точка входа
├── pkg/
│   ├── parser/                     # Парсер SQL
│   ├── executor/                   # Исполнитель запросов
│   ├── storage/                    # Хранилище данных
│   ├── index/                      # Индексы
│   ├── transaction/                # Транзакции и MVCC
│   ├── wal/                        # Write-Ahead Log
│   ├── optimizer/                  # Оптимизатор запросов
│   ├── catalog/                    # Метаданные (схема БД)
│   ├── network/                    # Сетевой протокол (клиент-сервер)
│   ├── auth/                       # Аутентификация и права
│   └── replication/                # Репликация (на будущее)
├── api/
│   ├── grpc/                       # gRPC API
│   ├── rest/                       # REST API
│   └── websocket/                  # WebSocket для real-time
├── ui/
│   └── dashboard/                  # Web UI (React/Vue)
├── tests/
├── docs/
├── go.mod
├── go.sum
└── Makefile
📦 Детальное описание модулей
1. catalog — Метаданные и схемы
Хранение информации о БД, таблицах, колонках, типах данных

Constraints (PRIMARY KEY, FOREIGN KEY, UNIQUE, NOT NULL, CHECK)

Data dictionary

Версионирование схемы (миграции)

2. parser — Парсер SQL
Лексер (токенизация)

Рекурсивно-нисходящий парсер

AST (Abstract Syntax Tree) для всех типов запросов

Поддержка стандарта SQL-92 + расширения:

SELECT (с JOIN, подзапросами, UNION, GROUP BY, HAVING, ORDER BY)

INSERT, UPDATE, DELETE

CREATE/DROP/ALTER TABLE

CREATE/DROP INDEX

BEGIN/COMMIT/ROLLBACK

EXPLAIN

3. executor — Исполнитель
План выполнения запроса (execution plan)

Pipeline execution (цепочка операторов)

Операторы:

TableScan, IndexScan

Filter, Project

Join (NestedLoop, HashJoin)

Sort, Limit

Aggregate (COUNT, SUM, AVG, MIN, MAX)

Контекст выполнения (транзакция, пользователь)

4. optimizer — Оптимизатор
Cost-based optimizer

Статистика таблиц (кардинальность, гистограммы)

Выбор индекса

Порядок join'ов

Predicate pushdown

5. storage — Движок хранения
Page Manager — страничная организация (4KB страницы)

Buffer Pool — кэш страниц в памяти (LRU)

Heap File — хранение строк в страницах

Формат строки (slotted page)

Поддержка разных storage engines:

Heap (по умолчанию)

Columnar (для аналитики) — на будущее

In-Memory (для временных таблиц)

6. index — Индексы
B+Tree индекс (основной)

Hash индекс (для точного поиска)

Поддержка составных ключей

Уникальные индексы

Partial indexes (WHERE clause)

Построение индекса в фоне

7. transaction — Транзакции и MVCC
ACID гарантии

MVCC (Multi-Version Concurrency Control):

Версии строк с метками времени

Snapshot isolation

Read committed / Repeatable read / Serializable

Transaction manager

Deadlock detection

Two-phase locking (опционально)

8. wal — Write-Ahead Logging
Журнал изменений для durability

Redo/Undo логи

Checkpoint механизм

Восстановление после сбоя

WAL shipping для репликации

9. network — Сетевой протокол
TCP сервер с пулом соединений

Wire protocol (бинарный протокол)

Prepared statements

Компрессия данных

Keep-alive

10. auth — Безопасность
Пользователи и роли

GRANT/REVOKE

Аутентификация (пароль, сертификаты)

Аудит запросов

11. replication — Репликация (фаза 2)
Master-slave

Синхронная/асинхронная

Консенсус (Raft) для multi-master

Автоматический failover

🔌 API и интерфейсы
1. Локальный Embedded API
go
db, _ := sqldb.Open("mydb")
result := db.Execute("SELECT * FROM users WHERE age > 18")
2. gRPC API (высокая производительность)
Бинарный протокол

Потоковая передача результатов

Подходит для микросервисов

3. REST API (для веб-приложений)
POST /query — выполнение SQL

GET /tables — список таблиц

GET /tables/{name}/schema — схема таблицы

Поддержка пагинации

4. WebSocket API (real-time)
Подписка на изменения (CDC — Change Data Capture)

Live queries

Notifications

5. Драйверы
Go database/sql driver

ODBC/JDBC (на будущее)

Python, Node.js клиенты

🖥️ Web UI Dashboard
На React/Vue:

Query editor с подсветкой синтаксиса и автодополнением

Table browser — просмотр данных как в phpMyAdmin

Schema designer — визуальное проектирование таблиц

Performance monitor — графики запросов, блокировок, кэша

User management

Backup/Restore

📅 Фазы разработки (Roadmap)
Фаза	Содержание	Срок
v0.1	In-memory хранилище, парсер SELECT/INSERT, без транзакций	2 недели
v0.2	Дисковое хранилище, WAL, базовые транзакции	2 недели
v0.3	B+Tree индексы, оптимизатор запросов	2 недели
v0.4	MVCC, уровни изоляции	2 недели
v0.5	JOIN, подзапросы, агрегаты	2 недели
v0.6	Сетевой протокол, клиент-сервер	2 недели
v0.7	gRPC/REST API	2 недели
v0.8	Web UI первая версия	2 недели
v1.0	Стабильная версия, бенчмарки	4 недели
v1.1+	Репликация, шардинг	TBD
🛠️ Ключевые принципы
Интерфейсы везде — StorageEngine, IndexType, ExecutorNode — чтобы можно было подменять реализации

Конкурентность — читатели не блокируют читателей (MVCC)

Тестируемость — каждый пакет тестируется изолированно

Расширяемость — плагины для storage engines, index types, функций

Observability — Prometheus метрики, логирование, tracing