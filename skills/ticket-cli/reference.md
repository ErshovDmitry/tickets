---
name: ticket-cli-reference
version: "1.20"
created: 2026-09-04
modified: 2026-11-25
type: reference
tags: [ticket, cli, help, reference]
description: "Verbatim `ticket help` output (RU) — full command reference."
---

# ticket help (verbatim)

Re-synced with the embedded RU template `internal/cli/templates/usage.ru.txt` on 2026-11-25 (T-0240 final pass: the exit-code paragraph now names `comment`/`section` alongside `set`/`edit` as refusing a deleted ticket, matching the `deletedGuard` all four commands share); before that on 2026-11-25 (T-0240 review pass 2 split the `comment --json` refusal promise: unknown fields are listed with the expected format/example, only invalid JSON or trailing garbage carry a byte position); before that on 2026-11-25 (T-0240 added the `comment` and `section set` blocks: the payload source contract, strict `--json`/`--schema`, protected sections, the heading guard and the exit-code list; T-0241 added the `show` block's `--section`/`--json` lines: three modes, the section name contract and the JSON field list); before that on 2026-11-24 (T-0237 corrected the `set` comment rule: a resolved source is copied byte for byte and exactly one trailing LF or CRLF is stripped, while any remaining CR or LF — `x\n\n`, a lone trailing CR, an embedded break — is refused; the earlier "a CR or LF in it is refused" wording was wrong); before that on 2026-11-22 (T-0226 reworded the `TICKET_LOCK_TIMEOUT` line from the wall-clock wait to the holder-stall budget: no holder progress for the configured value fails the command, a working holder extends the wait up to 12×); previously on 2026-11-21 (T-0215 documented the `set` comment source rules in the `set` block: a single comment argument is a source (`-` — stdin, `@<file>` — file content, `@@имя` — file named "@имя"), two or more arguments are joined with a space and used literally, the content is copied byte for byte (its CR/LF-refusal sentence was corrected by T-0237), and the lone-marker trade-off is stated); the previous re-sync was on 2026-11-16 (T-0189 W3 added the `list` `deleted` mode and the `deleted/` notes to `show`/`set`/`edit`/`archive`/`recover`/`normalize`, T-0188 added the `list` `--type`/`--priority` filters to the syntax line and the list block, T-0187 added the `ticket edit` block to the command list); the template itself was re-synced on 2026-11-11 (T-0162 documented the conservative single-file no-op in the `recover` block and made the exit-1 line name a single file with ANOTHER name; T-0159 added the `recover` block and the `recover` mention in the exit-code line; T-0109 added the `-d` input rules and the Details format rule; T-0094 added the `TICKET_LOCK_TIMEOUT` paragraph, T-0095 removed the migration section); T-0173 added the `update-check` block and exit code 3 to the exit-code line; `TICKET_LANG` default is RU. The first line below is `ticket version <ver>` — a placeholder, not part of the template: the build injects the real version via `-ldflags -X ticket/internal/cli.version=<ver>` (`dev` when built without `-ldflags`). Read the actual value from `ticket version` or `git describe --tags --always`.

Maintenance: regenerate with `go run ./cmd/ticket help` after code changes, then replace the injected first line with the `ticket version <ver>` placeholder.

```text
ticket version <ver>
ticket — тикеты проекта (файлы T-NNNN-<status>.md в <проект>/tickets/).

  ticket init
      создать структуру тикетов: tickets/, tickets/archive/ и tickets/deleted/ (идемпотентно); учитывает -C/--tickets-dir и $TICKETS_DIR (по умолчанию: <текущий каталог>/tickets)
  ticket new "<кратко>" [-t BUG|OPS|TD|ENH] [-p low|normal|high] [-d "<подробности>"] [-w кто] [-P <проект>]
      создать тикет (статус open), печатает путь к файлу;
      -P переопределяет проект (по умолчанию = basename родителя tickets/);
      предупреждение в stderr если проект встречается впервые
      -d принимает детали: обычный текст — буквально, @<файл> — содержимое файла, @@имя — файл с именем "@имя", - — stdin;
      @ без пути — ошибка: тикет не создаётся, сообщение локализовано;
      содержимое переносится байт в байт, пустой файл или stdin — пустое значение Details: в тикете показывается блок-формат Details (то же, что при -d ""); ограничения на размер нет;
      формат Details — нумерованный список, по одному пункту на строку: N. <что> — <файл:строка>; <решение>
  ticket list [active|open|wip|done|closed|archive|deleted|all] [-P <проект>] [--type BUG|OPS|TD|ENH] [--priority low|normal|high] [--json]
      список тикетов; по умолчанию active (= open + wip);
      archive — закрытые тикеты, унесённые в архив;
      deleted — мягко удалённые тикеты в tickets/deleted/; все прочие режимы (active/open/wip/done/closed/archive/all) их скрывают, all по-прежнему только основной каталог;
      -P фильтрует по проекту (точное совпадение, регистр важен);
      --type/--priority фильтруют по типу и приоритету (точное совпадение, регистр важен), соединяются со статусом и -P по И;
      неверное значение --type/--priority — ошибка (код 1)
      колонка проекта отображается когда проекты в выводе различаются;
      --json выводит массив JSON (6 полей: number, status, type, priority, title, created); пусто = []
  ticket show <номер|имя-файла> [--section <имя>] [--json]
      показать тикет (ищет и в архиве, и в deleted/); неоднозначный номер — код 2 со всеми конфликтующими путями
      --section печатает только тело секции без заголовка (пустая/отсутствующая секция — код 0, пустой stdout; неизвестное имя — код 1)
      --json выводит один JSON-документ: number, status, type, priority, project, title, summary, created, who, details, user_comments, comments, journal[], unknown, unknown_encoding, warnings[]; статус берётся из имени файла
      режимы взаимоисключаемы; диагностика — только stderr; лишние позиционные аргументы — предупреждение (код 0) в plain и ошибка (код 1) в --section/--json
  ticket set <номер> <статус> ["комментарий"]
      сменить статус (сам переименует файл и допишет строку в журнал тикета);
      повторный set в тот же статус с комментарием допишет журнал
      (статус и имя файла не меняются), без комментария — ошибка;
      комментарий можно дать ОДНИМ аргументом: - — stdin, @<файл> — содержимое файла, @@имя — файл с именем "@имя";
      @ без пути как ОДИН аргумент комментария — ошибка (код 1), так же ошибка при отсутствующем файле и сбое чтения stdin — имя тикета, его байты и журнал не меняются;
      содержимое переносится байт в байт, и срезается ровно один завершающий LF или CRLF; любой другой CR или LF — второй завершающий LF, одиночный завершающий CR, перевод строки внутри текста — отклоняется (код 1), ничего не пишется;
      два или больше аргументов комментария соединяются пробелом и берутся буквально — ни файл, ни stdin не читаются;
      компромисс: одиночный "-" или "@..." никогда не будет буквальным комментарием, а два аргумента — не побайтовый выход (пробел вставляется);
      "-" читает stdin до конца ввода; на терминале ждёт EOF (Ctrl-D)
      перевод архивного тикета в open/wip возвращает его из архива в работу;
      deleted этим путём не назначается: явное имя статуса или тикет, чей файл лежит в deleted/, отклоняются — код 1, файл не меняется
  ticket edit <номер> [--title "<кратко>"] [--type BUG|OPS|TD|ENH] [--priority low|normal|high] [--project <проект>]
      правка краткого описания/типа/приоритета/проекта тикета на месте (статус и имя файла не меняются);
      архивный тикет правится в archive/ без возврата в работу, --project меняет только метаданные;
      только раздельные пары "--флаг значение": форма "--флаг=значение" и короткие флаги отклоняются; повтор — последнее значение;
      явно переданное неизменённое значение отклоняет всю правку (код 1, ничего не записывается); успех тихий (код 0);
      тикет, чей файл лежит в deleted/, отклоняется (код 1, ничего не записывается)
  ticket comment <номер> [-] [--file <путь>] [--json] [--schema]
      добавить текстовый блок в секцию «Комментарии» (никогда не замена); источник payload — ровно один из - (stdin) или --file <путь>;
      нет источника или оба источника — ошибка использования (код 1), терминал не читается; --file читается побайтово, пустой файл читается как пустой payload;
      --json трактует payload как строгий JSON {"text":"<payload>"} (единственное поле): неизвестные поля — код 1 со списком полей/форматом/примером, невалидный JSON или мусор после объекта — код 1 с позицией/форматом/примером, без отката к plain;
      --schema печатает JSON Schema в stdout (код 0, без lookup и записи) и несовместим с источником;
      только раздельные пары "--флаг значение" после позиционного номера; строка payload — markdown-заголовок (1-6 '#' затем пробел или конец строки) — отклоняется (код 1) с номером строки;
      пустой payload отклоняется (код 1); успех тихий (код 0), parse-предупреждения в stderr; тикет в deleted/ отклоняется, неоднозначный номер — код 2 со всеми конфликтующими путями
  ticket section set <номер> <секция> [-] [--file <путь>] [--json]
      заменить тело редактируемой секции; <секция> — канонический CLI-токен (защищённое или неизвестное имя отклоняется, код 1, со списком редактируемых имён);
      источник payload — ровно один из - (stdin) или --file <путь>; нет источника или оба — ошибка использования (код 1), терминал не читается;
      --file читается побайтово (пустой файл очищает секцию); --json трактует payload как строгий JSON {"text":"<payload>"} (единственное поле), неизвестные поля или невалидный JSON — код 1, без отката к plain;
      пустой payload очищает секцию (пустое краткое описание рендерится как заголовок); строка payload — markdown-заголовок — отклоняется (код 1) с номером строки;
      только раздельные пары "--флаг значение" после позиционных; успех тихий (код 0), parse-предупреждения в stderr;
      тикет в deleted/ отклоняется (код 1), неоднозначный номер — код 2 со всеми конфликтующими путями
  ticket archive [<номер>]
      перенести закрытые тикеты (done/closed) в archive/;
      без номера — все закрытые, с номером — один указанный;
      удалённые тикеты игнорируются: ни переносятся в архив, ни восстанавливаются из него
  ticket recover <номер> <файл-новый> [--yes]
      восстановить тикет после прерванной смены статуса, оставившей два файла на один номер;
      <файл-новый> — базовое имя файла, который сохраняется (например T-0001-done.md), не путь;
      без --yes печатает сохраняемый и удаляемый файлы и спрашивает yes/no;
      без --yes и без терминала отказывает и ничего не меняет;
      удаляется только устаревший старый файл, сохраняемый файл не перезаписывается;
      повторный запуск после успешного восстановления и единственный файл с выбранным именем — консервативный успех без изменений;
      recover лишь проверяет наличие нужного файла: он не может доказать, что это ранее сохранённый файл, а не одинокий устаревший,
      поэтому код 0 значит «нужное имя уже на диске», а не «восстановление произошло»
      код 2 — номер не восстановить однозначно (три файла и более, расходящееся содержимое, одинаковый статус в именах);
      выбор старого файла, файла вне пары или единственного файла с другим именем — код 1, дерево не меняется;
      пара после прерывания с участием deleted/ восстанавливается по тем же правилам — сохранить удалённый файл можно, выбор устаревшего источника по-прежнему код 1
  ticket delete <номер|имя-файла> [--yes] [--force]
      мягко удалить тикет в deleted/ (тело, метаданные и журнал сохраняются; добавляется одна строка журнала о переходе в deleted);
      <номер|имя-файла> — один ref: номер или точное базовое имя (например T-0001-open.md), не путь;
      без --force удаляется только open-тикет в основном каталоге; --force покрывает и прочие статусы основного каталога, и архив;
      без --yes печатает путь кандидата и спрашивает yes/no; без --yes и без терминала отказывает и ничего не меняет;
      --yes пропускает вопрос, но не обходит guard; дубликат номера или одинаковое базовое имя в нескольких каталогах — код 2 со всеми конфликтующими путями;
      точное базовое имя выбирает один файл из дубликата номера; уже удалён, конфликт цели или не найден — код 1, дерево не меняется;
      повтор после успешного удаления — код 1 (уже удалён), а не успех без изменений; успех печатает путь удалённого файла
  ticket normalize [-P <проект>] [--dry-run]
      разово привести формально устаревшие файлы к канонической разметке;
      переписываются только файлы, байты которых отличаются от канонических: имя и статус не меняются,
      журнал не дополняется (отдельной журнальной строки о миграции нет);
      -P ограничивает прогон одним проектом (точное совпадение, регистр важен);
      --dry-run печатает пути, которые были бы переписаны, и ничего не пишет;
      файлы, которые команда не берётся чинить, пропускаются с предупреждением (старые метки «- Статус:», неполные метаданные);
      удалённые тикеты тоже переписываются на месте (порядок обхода: main, затем archive, затем deleted)
  ticket update-check
      есть ли новый релиз на GitHub? (явная проверка; никогда автоматически); кэш на пользователя (24 ч), повтор не чаще чем раз в 1 ч; безопасна офлайн: сбой не блокирует другие команды

Глобальный флаг (перед командой, все команды):
  -C <путь>, --tickets-dir <путь>
      путь к самому каталогу тикетов (не к корню проекта); работает из любой текущей директории
  --verbose
      отладочный лог в stderr; TICKET_LOG=<путь> тоже включает лог и отправляет записи в этот файл (файл важнее флага);
      принимается только ДО имени команды; после help/version игнорируется; склеенная форма --verbose=x не распознаётся;
      stdout для записей лога никогда не используется; init игнорирует --verbose (логгер для init не создаётся)

Поиск каталога тикетов (первое совпадение выигрывает): -C/--tickets-dir → $TICKETS_DIR → восходящий поиск tickets/ от текущего каталога. Исключение: init использует <cwd>/tickets без восходящего поиска.

Блокировка каталога: TICKET_LOCK_TIMEOUT задаёт, сколько .lock может оставаться без признаков прогресса от держателя, прежде чем ticket вернёт ошибку (пока держатель работает, ожидание продлевается до 12× от заданного значения); значение — Go-длительность с обязательной единицей (30s, 2m, 500ms); пустое или невалидное значение молча даёт дефолт; 0 или меньше — одна попытка без ожидания; действует на new, set, edit и archive.

Коды выхода: 0 — успех; 1 — ошибка (неверные аргументы, тикет не найден, конфликт, ошибка ввода или хранилища, сбой проверки обновлений); 2 — неоднозначный номер тикета (команды show, set, edit, archive, new, recover, delete, comment, section): один номер принадлежит нескольким файлам — в stderr печатаются номер и конфликтующие файлы. Команда list при дубликатах печатает такое же предупреждение, но завершается с кодом 0; 3 — доступно обновление (команда update-check). Дубликаты номеров внутри `tickets/deleted/` подчиняются тем же правилам: `list` (в любом режиме) предупреждает и выходит с кодом 0, `show` выходит с кодом 2 со всеми конфликтующими путями, а `set`/`edit`/`comment`/`section` отказывают по удалённому тикету с кодом 1.

Статусы: open — новый; wip — в работе; done — исправлено; closed — отклонён/дубликат; deleted — мягко удалён (терминальный, read-only).
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
| 2026-10-29 | T-0109: `-d` input rules and the Details format rule missing in the help text | Re-synced with `internal/cli/templates/usage.ru.txt` | 1.5 |
| 2026-11-11 | T-0159: `ticket recover` missing from the help text | Re-synced with `internal/cli/templates/usage.ru.txt` (recover block, `recover` in the exit-code line) | 1.6 |
| 2026-11-11 | T-0162: the single-file no-op and the single-file-with-another-name exit 1 were undocumented | Re-synced with `internal/cli/templates/usage.ru.txt` (conservative no-op lines, `единственного файла с другим именем`) | 1.7 |
| 2026-11-16 | T-0188: the `list` `--type`/`--priority` filters were missing from the help text | Re-synced with `internal/cli/templates/usage.ru.txt` (list syntax line and the `--type`/`--priority` block) | 1.9 |
| 2026-11-16 | T-0187: `ticket edit` was missing from the help text | Re-synced with `internal/cli/templates/usage.ru.txt` (edit block) | 1.9 |
| 2026-11-16 | Merge of T-0188 and T-0187 | Rebase resolution: help text keeps both the `list` filters and the `edit` block; version bumped past both parents | 1.10 |
| 2026-11-21 | T-0215 (GitHub #35): the `set` comment source rules (single-argument `-`/`@file`/`@@name`, multi-argument literal join, CR/LF refusal) were missing from the help text | Re-synced with `internal/cli/templates/usage.ru.txt` (set block: one comment argument = a source, two or more = literal space-join, byte-exact copy, CR/LF refusal without mutation) | 1.12 |
| 2026-11-22 | T-0226: the `TICKET_LOCK_TIMEOUT` line was reworded (holder-stall budget instead of a wall-clock wait) | Re-synced with `internal/cli/templates/usage.ru.txt` (lock-timeout line); the version was not bumped in that commit | 1.13 |
| 2026-11-24 | T-0237: the `set` comment rule claimed a CR/LF is refused, but the binary strips exactly one trailing LF or CRLF and refuses only what remains | Re-synced with `internal/cli/templates/usage.ru.txt` (set block: byte-for-byte copy + one stripped trailing LF/CRLF, remaining CR/LF refused) | 1.15 |
| 2026-11-24 | The reference had drifted only in its metadata — the embedded help was already byte-identical to the binary built from HEAD | Verified the help block against `ticket help` from HEAD (`0712bb5`): identical; bumped the version, fixed `modified`, added the missed 2026-11-22 row | 1.14 |
| 2026-11-25 | T-0241: `show` gained `--section <name>` and `--json` (three mutually exclusive modes, the section name/alias contract, the JSON field list) | Re-synced with `internal/cli/templates/usage.ru.txt` (show block: syntax line + three mode lines) | 1.16 |
| 2026-11-25 | T-0240: the template gained the `comment` and `section set` blocks (payload source contract, strict `--json`/`--schema`, protected sections, heading guard, exit-code list) | Fence re-synced by T-0240 A2; this row credits the re-sync and refreshes the provenance prose; verified byte-identical to `ticket help` of the build `2.0.3-7-g240d10b` (minus the version line) | 1.17 |
| 2026-11-25 | T-0240 review pass 1: the `comment` block's "empty file is a valid empty payload" contradicted the exit-1 refusal and the ATX guard omitted the end-of-line case | Re-synced the fenced help with the corrected RU template (empty file read as an empty payload; heading guard "whitespace or end of line") | 1.18 |
| 2026-11-25 | T-0240 review pass 2: the `comment --json` line promised position/format/example for unknown fields, but the unknown-fields refusal carries no byte position | Re-synced the fenced help with the reworded RU template (unknown fields: field list/format/example; position only for invalid JSON/trailing garbage) | 1.19 |
| 2026-11-25 | T-0240 final pass (F5): the exit-code paragraph named only `set`/`edit` as refusing a deleted ticket, although `comment`/`section set` share the same `deletedGuard` | Re-synced the fenced help with the corrected RU template (the deleted-refusal phrase now names `set`/`edit`/`comment`/`section`); byte parity with `internal/cli/templates/usage.ru.txt` re-verified (`usage_registry_test`) | 1.20 |
