# SQL-DB: собственная СУБД на Go

Встроенная SQL база данных с полным стеком.

## Архитектура

```
sql-db/
├── cmd/                     # Точки входа
│   └── sqldb/main.go        # Серверное приложение
├── pkg/                     # Ядро СУБД
│   ├── parser/             # Парсер SQL
│   ├── executor/          # Исполнитель запросов
│   ├── storage/          # Хранилище данных
│   ├── index/            # Индексы
│   ├── transaction/      # Транзакции и MVCC
│   ├── wal/              # Write-Ahead Log
│   ├── optimizer/        # Оптимизатор запросов
│   ├── catalog/         # Метаданные (схема БД)
│   ├── network/         # Сетевой протокол
│   ├── auth/           # Аутентификация и права
│   └── replication/    # Репликация
├── api/                    # API слои
│   ├── grpc/            # gRPC API
│   ├── rest/            # REST API
│   └── websocket/      # WebSocket
├── ui/                    # Web UI
│   └── dashboard/      # Dashboard (React/Vue)
├── tests/                # Интеграционные тесты
├── docs/                # Документация
├── go.mod
└── Makefile
```

## Модули

### 1. catalog — Метаданные и схемы

- Хранение информации о БД, таблицах, колонках, типах данных
- Constraints: PRIMARY KEY, FOREIGN KEY, UNIQUE, NOT NULL, CHECK
- Data dictionary
- Версионирование схемы (миграции)

### 2. parser — Парсер SQL

- Лексер (токенизация)
- Рекурсивно-нисходящий парсер
- AST (Abstract Syntax Tree)
- Поддержка SQL-92 + расширения:
  - SELECT (с JOIN, подзапросами, UNION, GROUP BY, HAVING, ORDER BY)
  - INSERT, UPDATE, DELETE
  - CREATE/DROP/ALTER TABLE
  - CREATE/DROP INDEX
  - BEGIN/COMMIT/ROLLBACK
  - EXPLAIN

### 3. executor — Исполнитель

- План выполнения запроса
- Pipeline execution
- Операторы: TableScan, IndexScan, Filter, Project, Join, Sort, Limit, Aggregate
- Контекст выполнения (транзакция, пользователь)

### 4. optimizer — Оптимизатор

- Cost-based optimizer
- Статистика таблиц (кардинальность, гистограммы)
- Выбор индекса и порядка join'ов
- Predicate pushdown

### 5. storage — Движок хранения

- Page Manager (страничная организация, 4KB страницы)
- Buffer Pool (LRU кэш)
- Heap File (slotted pages)
- Поддержка Storage Engines:
  - Heap (по умолчанию)
  - Columnar (для аналитики)
  - In-Memory (для временных таблиц)

### 6. index — Индексы

- B+Tree индекс (основной)
- Hash индекс (для точного поиска)
- Составные и уникальные индексы
- Partial indexes

### 7. transaction — Транзакции и MVCC

- ACID гарантии
- MVCC (Multi-Version Concurrency Control)
- Snapshot isolation
- Уровни изоляции: Read committed, Repeatable read, Serializable
- Deadlock detection
- Two-phase locking

### 8. wal — Write-Ahead Logging

- Redo/Undo логи
- Checkpoint механизм
- Восстановление после сбоя

### 9. network — Сетевой протокол

- TCP сервер с пулом соединений
- Бинарный wire protocol
- Prepared statements, компрессия

### 10. auth — Безопасность

- Пользователи и роли
- GRANT/REVOKE
- Аутентификация (пароль, сертификаты)

### 11. replication — Репликация

- Master-slave (синхронная/асинхронная)
- Raft консенсус для multi-master
- Автоматический failover

---

## API и интерфейсы

### Embedded API

```go
db, _ := sqldb.Open("mydb")
result := db.Execute("SELECT * FROM users WHERE age > 18")
```

### REST API

```bash
POST /query        # выполнение SQL
GET  /tables      # список таблиц
GET  /tables/:name/schema  # схема таблицы
```

### WebSocket

- Подписка на изменения (CDC)
- Live queries
- Notifications

### Драйверы

- Go: `database/sql` driver
- Python, Node.js клиенты

---

## Web UI Dashboard

- Query editor с подсветкой синтаксиса
- Table browser
- Schema designer
- Performance monitor
- Backup/Restore

---

## Roadmap

| Версия | Содержание | Срок |
|--------|------------|------|
| v0.1 | In-memory хранилище, парсер SELECT/INSERT | 2 недели |
| v0.2 | Дисковое хранилище, WAL, базовые транзакции | 2 недели |
| v0.3 | B+Tree индексы, оптимизатор | 2 недели |
| v0.4 | MVCC, уровни изоляции | 2 недели |
| v0.5 | JOIN, подзапросы, агрегаты | 2 недели |
| v0.6 | Сетевой протокол, клиент-сервер | 2 недели |
| v0.7 | gRPC/REST API | 2 недели |
| v0.8 | Web UI первая версия | 2 недели |
| v1.0 | Стабильная версия, бенчмарки | 4 недели |
| v1.1+ | Репликация, шардинг | TBD |

---

## Принципы

- **Интерфейсы везде** — подмена реализаций (StorageEngine, IndexType, ExecutorNode)
- **Конкурентность** — читатели не блокируют читателей (MVCC)
- **Тестируемость** — изолированное тестирование пакетов
- **Расширяемость** — плагины для storage engines, index types
- **Observability** — Prometheus метрики, логирование