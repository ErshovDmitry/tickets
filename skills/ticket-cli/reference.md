---
name: ticket-cli-reference
version: "1.4"
created: 2026-09-04
modified: 2026-10-27
type: reference
tags: [ticket, cli, help, reference]
description: "Verbatim `ticket help` output (RU) — full command reference."
---

# ticket help (verbatim)

Captured from `ticket` v1.2.3 (PATH binary) help on 2026-09-09 (`TICKET_LANG` default RU), then re-synced with the embedded RU template `internal/cli/templates/usage.ru.txt` on 2026-10-27 (T-0094 added the `TICKET_LOCK_TIMEOUT` paragraph, T-0095 removed the migration section). The first line is the runtime version line, not part of the template.

Maintenance: regenerate with `go run ./cmd/ticket help` after code changes; the first output line shows the binary version.

```text
ticket version 1.2.3
ticket — тикеты проекта (файлы T-NNNN-<status>.md в <проект>/tickets/).

  ticket init
      создать структуру тикетов в текущем каталоге: tickets/ и tickets/archive/ (идемпотентно)
  ticket new "<кратко>" [-t BUG|OPS|TD|ENH] [-p low|normal|high] [-d "<подробности>"] [-w кто] [-P <проект>]
      создать тикет (статус open), печатает путь к файлу;
      -P переопределяет проект (по умолчанию = basename родителя tickets/);
      предупреждение в stderr если проект встречается впервые
  ticket list [active|open|wip|done|closed|archive|all] [-P <проект>] [--json]
      список тикетов; по умолчанию active (= open + wip);
      archive — закрытые тикеты, унесённые в архив;
      -P фильтрует по проекту (точное совпадение, регистр важен);
      колонка проекта отображается когда проекты в выводе различаются;
      --json выводит массив JSON (6 полей: number, status, type, priority, title, created); пусто = []
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

Глобальный флаг (перед командой, все команды кроме init):
  -C <путь>, --tickets-dir <путь>
      путь к самому каталогу тикетов (не к корню проекта); работает из любой текущей директории

Поиск каталога тикетов (первое совпадение выигрывает): -C/--tickets-dir → $TICKETS_DIR → восходящий поиск tickets/ от текущего каталога.

Блокировка каталога: TICKET_LOCK_TIMEOUT задаёт, сколько ticket ждёт захват .lock, прежде чем вернуть ошибку; значение — Go-длительность с обязательной единицей (30s, 2m, 500ms); пустое или невалидное значение молча даёт дефолт; 0 или меньше — одна попытка без ожидания; действует на new, set и archive.

Статусы: open — новый; wip — в работе; done — исправлено; closed — отклонён/дубликат.
Типы: BUG — поломка; OPS — инцидент/обслуживание; TD — техдолг; ENH — улучшение.
Секреты (пароли, токены, ключи) в тикеты НЕ писать.
```

## File History & Known Issues

| Date | Issue | Fix Applied | Version |
|------|-------|-------------|---------|
| 2026-09-04 | Initial version | Captured verbatim help output | 1.0 |
| 2026-09-07 | Missing `-P` option, stale version, blank line | Regenerated after FIX 2+4 (T-0056) | 1.1 |
| 2026-09-09 | Stale vs v1.2.3: `list --json` (T-0081) missing | Regenerated from v1.2.3 help | 1.3 |
| 2026-10-27 | T-0094: `TICKET_LOCK_TIMEOUT` missing; T-0095: migration/sed section removed | Re-synced with `internal/cli/templates/usage.ru.txt` | 1.4 |
