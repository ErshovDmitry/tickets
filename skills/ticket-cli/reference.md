---
name: ticket-cli-reference
version: "1.0"
created: 2026-09-04
modified: 2026-09-04
type: reference
tags: [ticket, cli, help, reference]
description: "Verbatim `ticket help` output (RU) — full command reference."
---

# ticket help (verbatim)

Captured from `./tickets/bin/ticket` v1.0.0-8-g0d28fad on 2026-09-04 (`TICKET_LANG` default RU).

Maintenance: regenerate with `./tickets/bin/ticket` (no args) after rebuilding the binary; the first output line shows the binary version — re-check it then.

```
ticket version 1.0.0-8-g0d28fad
ticket — тикеты проекта (файлы T-NNNN-<status>.md в <проект>/tickets/).

  ticket new "<кратко>" [-t BUG|OPS|TD|ENH] [-p low|normal|high] [-d "<подробности>"] [-w кто]
      создать тикет (статус open), печатает путь к файлу
  ticket list [active|open|wip|done|closed|archive|all]
      список тикетов; по умолчанию active (= open + wip);
      archive — закрытые тикеты, унесённые в архив
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

## File History & Known Issues

| Date | Issue | Fix Applied | Version |
|------|-------|-------------|---------|
| 2026-09-04 | Initial version | Captured verbatim help output | 1.0 |
