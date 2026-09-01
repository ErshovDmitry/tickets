# ticket

Кроссплатформенная (Windows / Linux / macOS) CLI-система тикетов на Go.

Один статический бинарник `ticket`, ноль зависимостей в рантайме. Тикеты — обычные Markdown-файлы `T-NNNN-<status>.md` в каталоге `tickets/` проекта: читаются человеком без инструментов, единственный источник правды — файлы, база данных не нужна.

Кроссплатформенный аналог bash-версии: [init-tickets.md](https://gitlab.com/ai-dmitry/promts/-/blob/main/init-tickets.md).

## Использование

```
ticket new "<кратко>" [-t BUG|OPS|TD|ENH] [-p low|normal|high] [-d "<подробности>"] [-w кто]
ticket list [active|open|wip|done|closed|all]
ticket show <номер|имя-файла>
ticket set <номер> <статус> ["комментарий"]
```

Статусы: `open` → `wip` → `done` / `closed`. Статус хранится в имени файла; смена статуса — только через `ticket set` (инструмент сам переименует файл и допишет журнал тикета).

Бинарник кладётся в `<проект>/tickets/bin/` и сам находит каталог тикетов из любой текущей директории; также ищет `tickets/` вверх от текущей директории и понимает переменную `TICKETS_DIR`.

## Сборка

```bash
go build ./cmd/ticket                                        # текущая платформа
GOOS=windows GOARCH=amd64 go build -o dist/ticket.exe ./cmd/ticket
```

## Статус

🚧 В разработке.
