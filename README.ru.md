[English](README.md) | **Русский**

# ticket

Кроссплатформенная (Windows / Linux / macOS) CLI-система тикетов на Go.

## 🎬 Видео

[![Смотреть на YouTube](https://img.youtube.com/vi/UW6O2zhEPwc/hqdefault.jpg)](https://youtu.be/UW6O2zhEPwc)

- [YouTube](https://youtu.be/UW6O2zhEPwc)
- [Дзен](https://dzen.ru/video/watch/6a9a61d4b6339503daf3830e)
- [Rutube](https://rutube.ru/video/29ab80e52b2d3c7afaef1eae0d0a0565/)

## Что это

- Тикеты — обычные Markdown-файлы `T-NNNN-<status>.md` в каталоге `tickets/` проекта. Единственный источник правды — файлы: читаются человеком без инструментов (`cat`, редактор, `git diff`), база данных не нужна.
- Один статический бинарник `ticket` (`ticket.exe` на Windows), собирается с `CGO_ENABLED=0`, ноль runtime-зависимостей.
- Кроссплатформенный аналог bash-версии [init-tickets.md](https://gitlab.com/ai-dmitry/promts/-/blob/main/init-tickets.md).

## Установка

Готовый бинарник кладётся в `<проект>/tickets/bin/` — оттуда он сам находит каталог тикетов и работает из любой текущей директории. Порядок резолюции (первое совпадение выигрывает):

1. `$TICKETS_DIR` — явный override;
2. восходящий поиск от текущей директории каталога с именем `tickets` (как у git);
3. относительно бинаря: `<каталог-бинаря>/..` — стандартная раскладка `tickets/bin/ticket`.

### Конфигурация (env)

| Переменная | Назначение |
|------------|------------|
| `TICKETS_DIR` | явный override каталога тикетов |
| `TICKET_WHO` | автор тикета; цепочка `TICKET_WHO → USER → USERNAME → agent` |
| `TICKET_LANG` | язык справки `ticket help` и файлов, создаваемых `ticket new`; цепочка `TICKET_LANG → LC_ALL → LANG`, по умолчанию **RU** (EN — opt-in: `TICKET_LANG=en`) |

## Сборка из исходников

Требуется Go >= 1.26 (единственная внешняя зависимость — `golang.org/x/sys`, нужна для блокировки файлов на Windows).

Версия бинаря — из git-тега (фолбэк `dev`); задайте переменную один раз перед сборкой:

```bash
VER=$(git describe --tags --always 2>/dev/null | sed 's/^v//'); VER=${VER:-dev}
```

Текущая платформа:

```bash
CGO_ENABLED=0 go build -ldflags "-X ticket/internal/cli.version=$VER" -o tickets/bin/ticket ./cmd/ticket
```

Кросс-компиляция (везде `CGO_ENABLED=0`, формат `GOOS=... GOARCH=... go build -o dist/<имя> ./cmd/ticket`):

| Цель | Команда |
|------|---------|
| linux/amd64 | `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket-linux-amd64 ./cmd/ticket` |
| linux/arm64 | `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket-linux-arm64 ./cmd/ticket` |
| windows/amd64 | `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket.exe ./cmd/ticket` |
| windows/arm64 | `CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket-windows-arm64.exe ./cmd/ticket` |
| darwin/amd64 | `CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket-darwin-amd64 ./cmd/ticket` |
| darwin/arm64 | `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket-darwin-arm64 ./cmd/ticket` |

> ⚠️ На ФС без поддержки hardlinks (FAT/exFAT на съёмных носителях) команды `set` и `archive` падают с ошибкой `link` — это fail-safe, данные не теряются; держите `tickets/` на NTFS или POSIX-ФС, подробности в [DEPLOY.md](DEPLOY.md).

Если проект под git — решите, коммитить `tickets/` вместе с проектом или добавить его в `.gitignore`.

## Использование

Быстрый старт:

```bash
# создать тикет (статус open), печатает путь к файлу
ticket new "Поломка экспорта" -t BUG -p high -d "Падает на больших файлах" -w ivan

# -P переопределяет проект (по умолчанию = basename родителя tickets/);
# предупреждение в stderr, если имя проекта встречается впервые
ticket new "Чужой проект" -P otherproj

# список тикетов: по умолчанию active (= open + wip)
ticket list
ticket list all        # включая done/closed
ticket list archive    # только архив
ticket list all -P otherproj   # фильтр по проекту (точное совпадение, регистр важен)

# показать тикет (ищет и в архиве)
ticket show 7

# сменить статус: open → wip → done / closed
ticket set 7 wip "взял в работу"
ticket set 7 done "починено, тесты зелёные"

# перенести закрытые тикеты (done/closed) в tickets/archive/
ticket archive         # все закрытые
ticket archive 7       # один указанный

# версия бинарника (dev, если собран без -ldflags)
ticket version
```

Пример вывода `ticket list all` — колонка проекта появляется, когда проекты в выводе различаются:

```
T-0001  tmp         open     BUG: Экспорт падает
T-0002  tmp         open     ENH: Broken export
T-0003  otherproj   open     BUG: Cross proj
```

С фильтром `-P` колонка проекта исчезает (проект один):

```
T-0003  open     BUG: Cross proj
```

Полная справка по командам — вербатим вывод `ticket help` (первая строка — версия бинаря, `dev`, если собран без `-ldflags`):

```
ticket version dev
ticket — тикеты проекта (файлы T-NNNN-<status>.md в <проект>/tickets/).

  ticket new "<кратко>" [-t BUG|OPS|TD|ENH] [-p low|normal|high] [-d "<подробности>"] [-w кто] [-P <проект>]
      создать тикет (статус open), печатает путь к файлу;
      -P переопределяет проект (по умолчанию = basename родителя tickets/);
      предупреждение в stderr если проект встречается впервые
  ticket list [active|open|wip|done|closed|archive|all] [-P <проект>]
      список тикетов; по умолчанию active (= open + wip);
      archive — закрытые тикеты, унесённые в архив;
      -P фильтрует по проекту (точное совпадение, регистр важен);
      колонка проекта отображается когда проекты в выводе различаются
  ticket show <номер|имя-файла>
      показать тикет (ищет и в архиве)
  ticket set <номер> <статус> ["комментарий"]
      сменить статус (сам переименует файл и допишет строку в журнал тикета);
      повторный set в тот же статус с комментарием допишет журнал
      (статус и имя файла не меняются), без комментария — ошибка;
      перевод архивного тикета в open/wip возвращает его из архива в работу
  ticket archive [<номер>]
      перенести закрытые тикеты (done/closed) в archive/;
      без номера — все закрытые, с номером — один указанный

Статусы: open — новый; wip — в работе; done — исправлено; closed — отклонён/дубликат.
Типы: BUG — поломка; OPS — инцидент/обслуживание; TD — техдолг; ENH — улучшение.
Секреты (пароли, токены, ключи) в тикеты НЕ писать.

Миграция старых тикетов: старый формат с русскими заголовками не поддерживается новыми версиями; выполните один раз над tickets/*.md:

sed -i -E \
 -e 's/^## Кратко$/## Summary (Кратко)/' \
 -e 's/^## Подробности$/## Details (Подробности)/' \
 -e 's/^## Комментарии от пользователя$/## User comments (Комментарии от пользователя)/' \
 -e 's/^## Комментарии$/## Comments (Комментарии)/' \
 -e 's/^## Журнал$/## Journal (Журнал)/' \
 -e 's/^- Статус:/- Status (Статус):/' \
 -e 's/^- Приоритет:/- Priority (Приоритет):/' \
 -e 's/^- Создан: (.*) · кем: (.*)$/- Created (Создан): \1 · by (кем): \2/' \
 -e 's/^- Проект:/- Project (Проект):/' tickets/*.md
```

> Примечание: команда миграции написана для GNU sed (Linux); на macOS используйте `sed -i ''`, на Windows — Git Bash или WSL.

## Формат файла тикета

- Имя файла: `T-NNNN-<status>.md`; номер выдаётся атомарно через OS-локи и не переиспользуется.
- Статус хранится в имени файла. Вручную файлы не переименовывать: смену статуса делает `ticket set` (сам переименует файл и допишет журнал тикета).
- Жизненный цикл: `open` (новый) → `wip` (в работе) → `done` (исправлено) / `closed` (отклонён/дубликат).
- Типы в заголовке: `BUG` — поломка; `OPS` — инцидент/обслуживание; `TD` — техдолг; `ENH` — улучшение.
- i18n: формат файла выбирается по `TICKET_LANG` в момент создания. Русская локаль даёт двуязычные метки (`## Summary (Кратко)`, `- Status (Статус): open`), английская — без перевода (`## Summary`, `- Status: open`). Старые файлы с чисто русскими заголовками новыми версиями не поддерживаются — см. однострочник миграции в выводе `ticket help`.
- `ticket new` без `-d` вставляет в `Details` HTML-комментарий-подсказку (что найдено, где — файл:строка, логи, как воспроизвести, предложение фикса).
- Архив: `ticket archive` переносит закрытые тикеты в `tickets/archive/`; `list archive` смотрит архив; `show` и `set` работают с архивом, перевод архивного тикета в `open`/`wip` возвращает его в работу.
- Секреты (пароли, токены, ключи) в тикеты НЕ писать.

Пример тикета (`ticket new "Экспорт падает" -t BUG -p high -d "Падает на больших файлах" -w ivan`, локаль RU):

```markdown
# T-0001 · BUG: Экспорт падает

- Status (Статус): open
- Priority (Приоритет): high
- Created (Создан): 2026-09-04 05:54 · by (кем): ivan
- Project (Проект): tmp

## Summary (Кратко)
Экспорт падает

## Details (Подробности)
Падает на больших файлах

## User comments (Комментарии от пользователя)

## Comments (Комментарии)

## Journal (Журнал)
- 2026-09-04 05:54 — тикет создан (ivan).
```

Секции: `Summary` — кратко; `Details` — подробности (файл:строка, логи, воспроизведение); `User comments` — пишет пользователь, агент читает перед работой над тикетом и туда не пишет; `Comments` — рабочие заметки агента; `Journal` — журнал, дописывается автоматически командами `new`/`set`.

## Интеграция в AGENTS.md проекта

Вставьте блок ниже в `AGENTS.md` проекта, использующего `tickets/` (если файла нет в корне проекта — создайте; пути относительные — от корня проекта; бинарник сам найдёт каталог тикетов независимо от текущей директории):

```markdown
## Тикеты (`tickets/`)

- НА СТАРТЕ сессии: `./tickets/bin/ticket list` — если есть открытые тикеты, кратко напомни о них пользователю при первом же ответе.
- Нашёл во время работы проблему, ошибку или что-то подозрительное — СРАЗУ создай тикет: `./tickets/bin/ticket new "<кратко>" -t BUG|OPS|TD|ENH -p low|normal|high -d "<подробности>"`. НЕ молчи, даже если удалось обойти/починить по месту. В подробностях указывай файл:строка.
- Смена статуса — только через `./tickets/bin/ticket set <номер> <статус> "<комментарий>"` (бинарник сам переименует файл и допишет журнал тикета); вручную файлы тикетов не переименовывать.
- Перед работой над тикетом прочти его секцию «User comments (Комментарии от пользователя)» — замечания оставляет пользователь; агент туда не пишет. Свободные рабочие заметки агента — в «Comments (Комментарии)».
- Тесты/smoke-прогоны `ticket` — ТОЛЬКО в песочнице (временный каталог через `$TICKETS_DIR`), НИКОГДА в боевом `tickets/`.
- Закрытых тикетов (`done`/`closed`) стало много — перенеси их в архив: `./tickets/bin/ticket archive`. Архивные не теряются: `ticket list archive`, `show`/`set` работают и с ними, reopen возвращает тикет в работу.
- В тикеты НЕ писать секреты (пароли, токены, ключи).
```

## Скилл для агентов

В репозитории поставляется скилл для агентов в `skills/ticket-cli/` (`SKILL.md` + `reference.md`) — обучает ИИ-агентов рабочему процессу с тикетами: команды, жизненный цикл и подводные камни. Установка — скопировать каталог в `~/.agents/skills/`.

## Разработка

```bash
gofmt -l . && go vet ./...      # обязательно чисто перед коммитом
go test ./...                   # обязательно зелёное перед отметкой done
```

Локальная сборка — см. [Сборка из исходников](#сборка-из-исходников). Для Windows: кросс-компиляция `dist/ticket.exe` (windows/amd64, таблица выше), затем прогон `scripts/smoke-windows.ps1` на Windows-хосте (вручную по SSH). Smoke проверяет `new`/`list`/`show`/`set`, exe-relative резолюцию из чужой рабочей директории, override `TICKETS_DIR` и параллельный `new` ×5 (пять уникальных последовательных номеров через OS-лок) — целиком внутри временной песочницы, репозиторий и данные не затрагиваются.

## Ссылки

- bash-оригинал: [init-tickets.md](https://gitlab.com/ai-dmitry/promts/-/blob/main/init-tickets.md)
- `AGENTS_ARCHITECTURE.md` — архитектура (локальный файл, в репозиторий не публикуется)
- `DEPLOY.md` — сборка, требования к файловой системе

## Статус

**v1 done.**

## Что дальше

Есть мысли о дальнейшем развитии ticket, но они будут опубликованы позже, в комплексном виде. Когда будет что показать — здесь появится ссылка.
