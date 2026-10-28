# Как добавить новый язык в tickets

> Чек-лист по реструктуризации T-0038 (один файл данных на язык вместо хардкода
> в `internal/domain/i18n.go`). После неё добавление языка = два файла,
> правка Go-кода не требуется.

## Шаг 1. Файл данных языка

Создать `internal/domain/langs/<код>.json`, где `<код>` — строчный код языка
ISO 639-1 (`de`, `fr`, …). Файл встраивается в бинарь через
`//go:embed langs/*.json`; реестр языков строится автоматически в `init()`
из всех `*.json` в каталоге.

Проще всего скопировать `langs/ru.json` или `langs/en.json` и перевести
значения. Структура — все 10 полей обязательны, кодировка UTF-8:

```json
{
  "headers": [
    "<заголовок секции Summary>",
    "<заголовок секции Details>",
    "<заголовок секции User Comments>",
    "<заголовок секции Comments>",
    "<заголовок секции Journal>"
  ],
  "statusFmt": "<формат строки статуса, 1 %s>",
  "priorityFmt": "<формат строки приоритета, 1 %s>",
  "projectFmt": "<формат строки проекта, 1 %s>",
  "createdFmt": "<формат строки даты создания, 2 %s: дата, автор>",
  "detailsStub": "<текст-заглушка пустых Подробностей>",
  "userCommentsStub": "<текст-заглушка секции замечаний пользователя>",
  "journalCreation": "<строка журнала при создании, 2 %s: дата, кто>",
  "journalTransition": "<формат строки журнала при смене статуса, 3 %s: дата, из, в>",
  "journalArchive": "<строка журнала при архивировании, 2 %s: дата, кто>"
}
```

При загрузке валидируются `len(headers) == 5` и число `%s` в каждом форматном
поле (см. комментарии выше). С T-0107 паники при старте нет — бинарь
сообщает об ошибке и продолжает работу:

- битый **необязательный** язык (например, `de.json`) → одна строка в stderr
  (`ticket: i18n initialization degraded: parse langs/de.json: …`), этот язык
  пропускается, локаль считается неизвестной (→ EN), команды работают, код
  возврата 0;
- битый **обязательный** `en.json` → одна строка в stderr
  (`ticket: i18n initialization failed: parse langs/en.json: …`), stdout пуст,
  код возврата 1.

## Шаг 2. Usage-текст справки

Создать `internal/cli/templates/usage.<код>.txt` — скопировать
`usage.ru.txt` или `usage.en.txt` и перевести, сохраняя структуру
(первая строка — «ticket — …»; строку версии дописывает код, в файле её нет).

🔴 **Перед smoke бинарь обязательно пересобрать.** И файл данных языка
(`internal/domain/langs/<код>.json`), и usage-шаблон
(`internal/cli/templates/usage.<код>.txt`) встраиваются в бинарь на этапе
сборки (`//go:embed`), поэтому старый бинарь физически не содержит только
что добавленных файлов. Проверено эмпирически: после добавления
`templates/usage.de.txt` старый бинарь продолжал печатать
`usage: selected template for language "de" (templates/usage.de.txt): …` и
возвращать код 1 — ровно как при действительно отсутствующем файле; после
`go build` тот же запуск давал код 0 и пустой stderr. Без пересборки smoke
показывает ложную диагностику («нет usage-файла» / «язык не подхватил»).

```bash
go build -o /tmp/ticket-lang ./cmd/ticket   # бинарь для smoke
```

Оба файла обязательны (план §D, LOCK L5):

- только файл данных без usage-файла → справка **не падает**: при выводе
  help для этого языка код читает `templates/usage.<код>.txt`, не находит его
  и печатает английский `usage.en.txt` в stdout; в stderr уходит одна
  строка-диагностика
  (`usage: selected template for language "de" (templates/usage.de.txt): …`),
  код возврата **1** (поведение T-0107; раньше была паника). Команды с
  данными тикетов (`list`, `new`, `show`, `set`) при этом работают штатно —
  они локализуются по данным языка, а не по usage-файлу. Функциональный
  EN-fallback и текст ошибки зафиксированы тестом `TestUsageFromFallbackMatrix`
  (`internal/cli/run_order_test.go`): он вызывает `usageFrom` напрямую
  (внутренняя функция) и проверяет только её результат. Контракт уровня CLI
  (код возврата 1 и ровно одна строка в stderr) **для этого fallback-пути**
  этим прямым вызовом `usageFrom` не покрыт — приведённое выше поведение
  получено эмпирически;
- только usage-файл без данных → язык не попадёт в реестр, локаль
  будет считаться неизвестной (→ EN, см. «Fallback»).

## Что менять НЕ нужно

- `internal/domain/i18n.go` — реестр автоматически подхватывает
  `langs/*.json` в `init()`.
- `internal/cli/cli.go` — `langPrefix()` проверяет код языка через
  `IsKnownLang()` (запрос к реестру).
- После реструктуризации T-0038 добавление языка не требует правки
  Go-кода; ДО неё (текущий хардкод `dictRU`/`dictEN`) — требует.

## Проверка

1. Реестр подхватил новый файл. Тест `TestRegistryLoadsAllFiles`
   (`internal/domain/i18n_registry_test.go`) проверяет наличие RU и EN
   в реестре (`IsKnownLang`); новый файл он сам не подхватывает —
   добавьте в него аналогичную проверку для нового кода языка:

   ```bash
   go test ./internal/domain -run TestRegistryLoadsAllFiles
   ```

2. Все тесты зелёные:

   ```bash
   go test ./...
   ```

3. Smoke — только в песочнице (`TICKETS_DIR` на temp-каталог), не в боевом
   `tickets/`:

   ```bash
   mkdir -p /tmp/ticket-lang-tickets
   TICKET_LANG=de TICKETS_DIR=/tmp/ticket-lang-tickets /tmp/ticket-lang -h; echo $?   # usage-текст на новом языке
   TICKET_LANG=de TICKETS_DIR=/tmp/ticket-lang-tickets /tmp/ticket-lang list          # данные тикетов без сюрпризов
   ```

   Бинарь здесь — `/tmp/ticket-lang` из «Шага 2»: он свежесобран, старый
   бинарь не содержит новых встроенных файлов и даёт ложную диагностику.

   Если вывод по-английски — причин три, различаются они по коду возврата
   и stderr:

   - код возврата 0, stderr пуст → локаль не найдена в реестре языков
     (семантика Т-0036: неизвестная локаль → английский). Файл `<код>.json`
     отсутствует или не включён в реестр языков;
   - код возврата 0, в stderr одна строка
     `ticket: i18n initialization degraded: parse langs/de.json: …`
     → файл `<код>.json` есть, но невалиден (битый JSON или не прошла
     валидация). Язык пропущен, локаль считается неизвестной (→ EN), команды
     работают, код возврата 0 — так же, как в «Шаге 1». Причина — в самой
     строке stderr;
   - код возврата 1, в stderr строка
     `usage: selected template for language "de" (templates/usage.de.txt): …`
     → язык в реестре есть, но отсутствует
     `internal/cli/templates/usage.de.txt` (см. «Шаг 2»).

4. Fallback-проверки:

   ```bash
   TICKET_LANG=zz TICKETS_DIR=/tmp/ticket-lang-tickets /tmp/ticket-lang -h   # неизвестная локаль → английский
   TICKETS_DIR=/tmp/ticket-lang-tickets env -u TICKET_LANG -u LC_ALL -u LANG /tmp/ticket-lang -h   # пустой env → русский
   ```

## Fallback (итог)

- Код локали не найден в реестре языков → EN.
- `TICKET_LANG` / `LC_ALL` / `LANG` не установлены → RU
  (default, паритет с bash-версией).
- Язык есть в реестре, но нет `templates/usage.<код>.txt` → текст справки
  берётся из `usage.en.txt` (stdout), в stderr — одна строка-диагностика,
  код возврата 1 (T-0107; паники нет).

См. также: план T-0038 (wiki `project_tickets`, hub `ebb4f5f1`,
§A схема JSON, §B раскладка файлов, §D выбор языка, §F тесты),
`AGENTS_ARCHITECTURE.md` §4a.
