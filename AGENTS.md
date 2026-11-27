# AGENTS.md — tickets

## 🔴 Startup — Critical First

| Step | Tool | Action |
|------|------|--------|
| 1 | `wikidb` | 🔴 `wikidb({tool: "exec_sql_readonly", arguments: {query: "SELECT public.wikidb_env()"}})` — check environment |
| 2 | `wikidb` | 🔴 `wikidb({tool: "resources/read", arguments: {uri: "config://rules/gates"}})` — critical gates |
| 2a | `wikidb` | 🔴 `wikidb({tool: "resources/read", arguments: {uri: "config://skills/wikidb-init"}})` — init + ground rules (page types/status, fallback) |
| 3 | (Optional) | `resources/list` — full manifest ONLY when discovery is needed; prefer targeted `resources/read` |
| 4 | (Optional) | `resources/read` base_rules — extended reference, load on demand |
| 5 | `wikidb` | `wikidb({tool: "resources/read", arguments: {uri: "config://roles/{role}"}})` — role rules (root session: role = `orchestrator`) |
| 6 | (Optional) | `resources/read` for: `config://rules/title_convention`, `config://rules/role_aliases`, `config://rules/subagent_rules`, `config://skills/pi-agent`, `config://rules/sql_array_patterns`, `config://rules/action_item_visibility` |

> **Tool format:** `wikidb({tool: "mcp_tool_name", arguments: {_schema, ...}})` — one universal dispatcher. `resources/read` for config:// URIs. `exec_sql_readonly` for SQL. All wiki ops via `wikidb`.

## 🔴 Universal rules live in `knowledge` schema

All universal rules (base_rules, title_convention, role_aliases, `{role}` roles, skills) live in the **`knowledge`** schema. Always load with `_schema="knowledge"`:

```
config://rules/gates            (= config://rules/wf-default/gates)
config://rules/wf-default/base_rules
config://roles/coordinator
```

Wiki-схема **`project_tickets`** — ТОЛЬКО для планов/ревью/решений по гейтам (pass `_schema="project_tickets"` explicitly). Тикеты и рабочая документация — локальные файлы (см. ниже), в wiki НЕ дублируются.

🔴 **WikiDB будет заморожен** (решение пользователя 2026-11-12): заморозка ещё не выполнена, пользователь пишет новый проект-замену. Пользоваться WikiDB пока продолжаем (планы/ревью/гейты, `knowledge`), но **тикеты про WikiDB НЕ создавать нигде** — ни локальные в `tickets/`, ни в `hot.tickets`. Обнаружил проблему самой WikiDB (баг MCP-инструмента, identity-политика, дрейф расширения и т.п.) — обойди по месту и зафиксируй в рабочих заметках текущей задачи (страница плана/ревью или комментарий в тикете НАШЕЙ задачи); отдельный тикет про инфраструктуру WikiDB не писать. Чужие локальные тикеты про WikiDB, заведённые ранее (T-0145, T-0148, T-0155, T-0181, T-0182), закрыты 2026-11-12 без переноса.

Project-specific rules (if created) live in `project_tickets` — load with `_schema="project_tickets"`.

🔴 NEVER reference `project_wikidb` or any other project's schema. This project is self-contained; for infrastructure/structural info reference the common `knowledge` schema only.

## 🔴 PROCESS GATES — see `config://rules/gates`

| Trigger | MUST |
|---------|------|
| 3+ files OR 50+ lines | Write plan to wiki (`project_tickets`) |
| Code written (2+ files) | `subagent` code review |
| Review task | Fresh `subagent` (≠ you); wiki-страницу ревью создаёт оркестратор — у ревьюера read-only identity (см. Coordinator Requirements, п. 8) |
| Coordinator role | Delegate, never code/diff |
| Docker container removal | Ask user first |

## Project

**tickets** — кроссплатформенная (Windows / Linux / macOS) CLI-система тикетов на Go. Преемник bash-версии [`init-tickets.md`](https://gitlab.com/ai-dmitry/promts/-/blob/main/init-tickets.md): тикеты — обычные `.md`-файлы `T-NNNN-<status>.md` в `tickets/`, единственный источник правды — файлы, без БД. Один статический бинарник `ticket` (`ticket.exe`), атомарная выдача номеров через OS-локи.

Архитектура: см. **AGENTS_ARCHITECTURE.md** (рядом с этим файлом).

## Where Code Runs

- Development: этот ПК (erdmitry), репозиторий `/nob/dmitry/Progs/tickets`.
- Тест на Windows: хост **dser97** (SSH). 🔴 SSH-ключ НЕ хранить в репозитории и НЕ запрашивать у пользователя — ключ уже в TPM2-backed ssh-agent на этом ПК (erdmitry): сокет `/run/ssh-agent-tpm2/agent.sock`, юнит `ssh-agent-tpm2.service`, пароль ключа расшифровывает systemd через TPM2 (systemd-creds). В `~/.ssh/config` для `dser97.home` уже прописаны `IdentityAgent` + `IdentityFile` + `IdentitiesOnly` — просто `ssh dser97.home` / `scp`, ничего не настраивать и не модифицировать. Проверка: `SSH_AUTH_SOCK=/run/ssh-agent-tpm2/agent.sock ssh-add -l`. Подробности и диагностика — скилл `~/.agents/skills/ssh-agent-tpm2/SKILL.md` (🔴 загружать перед любой работой с dser97). Прецедент T-0086: агент отрапортовал «SSH blocked», хотя ломались PATH/кодировка — авторизация ни при чём; не повторять эту ошибочную диагностику. Сборка под Windows — кросс-компиляция (`GOOS=windows`), прогон — на dser97. Go на dser97 НЕ установлен и устанавливать не надо (решение пользователя 2026-09-09): прогон тестов — кросс-компиляция `GOOS=windows go test -c`, `scp` бинарей `*.test.exe` на dser97, запуск там. Кодировка вывода dser97 🔴: терминал wezterm НЕ поддерживает 8-битные кодовые страницы (wezterm/wezterm#1638, open — только UTF-8), а cmd на dser97 выводит русский в CP866 → мусор. Проверенные решения (2026-09-09): (1) `ssh dser97.home 'cmd /u /c <команда>' | iconv -f UTF-16LE -t UTF-8`; (2) `ssh dser97.home 'powershell -NoProfile -Command "[Console]::OutputEncoding=[Text.Encoding]::UTF8; <команда>"'` — чистый UTF-8 без iconv; (3) fallback `| iconv -f CP866 -t UTF-8`. `chcp 65001` НЕ работает через ssh-пайп (проверено). Вывод go test (*.test.exe) — UTF-8 сам по себе, мусор только от cmd/системных сообщений. 🔴 Флаги тест-бинаря в PowerShell обязательно в кавычках и с `=`: `& .\store.test.exe "-test.run=TestConcurrent_CreateAndSetStatus_NoJournalLoss_N50"` — без кавычек PowerShell рвёт `-test.run` и бинарь падает с `flag provided but not defined: -test`.

Worktree'ы агентов: конвенция — `/nob/dmitry/.wt/<ticket>` (id тикета в нижнем регистре), НЕ `$TMPDIR` и не внутри дерева репозитория: pi-lens 4.5.0 (`computeProbeHomeDir()`) пишет `.pi-lens-probe-home/` с логами проб прямо в worktree, если cwd внутри `os.tmpdir()` (или в корень `.claude/worktrees/<x>/`), из-за чего `git status` засоряется, а мусор может попасть в коммит через `git add -A`; подробности и команда создания — `docs/development.md` § «Agent worktrees».

## Current Agent Role

Orchestrator — assigned 2026-09-03 (до этого Coordinator, 2026-09-02). Оркестратор/координатор делегирует, код не пишет.

**Two-level hierarchy (user ruling 2026-09-05):** root session = **Orchestrator** (`config://roles/orchestrator`) — decomposes user requests into large tasks and delegates each to a coordinator-* subagent; may spawn a few worker subagents directly for small jobs; NEVER implements. **Coordinators** (coordinator-* agents, `config://roles/coordinator`) — run plan→review→code→review for ONE task, spawn all workers, never write code. Provider limits → `config://rules/wf-default/limits` (single source of truth; caps change often — re-read before every batch).

## 🔴 Coordinator Requirements (when role = coordinator)

> Bind coordinator-* instances; the orchestrator enforces them via coordinator briefs.

1) 🔴 Read skill manifests.
2) 🔴 The worker marked MECHANICAL-ONLY in `config://rules/wf-default/limits` — never use it for coding or complex tasks.
3) 🔴 STOP & THINK: how many atomic pieces can you split this into?
4) 🔴 **Split BEFORE review.** Any artifact >4000 chars (`PLAN_PAGE_MAX`, E3 gate) MUST be split into ≤4000-char pages BEFORE spawning reviewers — `get_page` truncates at `CHUNK_SIZE=50000` (flag `_truncated:true` is ignorable), so reviewing a monolith = risking verdicts on partial content. Empirically (2026-07-22): split-then-review found **3.5× more 🔴** than 3 cycles on the unsplit monolith, at lower cost. Load skill `split-before-review` (coordinator-only, `config://skills/split-before-review`) before ANY review batch.
5) 🔴 MUST use micro-agent work: split to ATOMIC, launch as ONE batch (1 atom = 1 reviewer; default-class reviewers for mechanical coverage, reserve flagship models for semantic landmines only — model names + batch caps per `config://rules/wf-default/limits`).
6) 🔴 DON'T RUSH — do it RIGHT. Review bar: 🔴=0 🟡=0 🔵=0.
7) 🔴 1 agent per review point. Single page → sequential, one agent per point.
8) 🔴 **Wiki-персист ревью делает оркестратор/координатор, а не ревьюер.** У субагентов-ревьюеров identity в WikiDB MCP — read-only: `create_page`/`link_pages` для них запрещены (default-deny по allowlist инструментов), поэтому отчёт ревьюера в wiki сам не попадёт. Обязательный порядок: (1) в бриф ревьюеру — требование подготовить в ответе ТЕКСТ страницы (title, type, tags, `metadata.review_of`/`metadata.reviewer`, список связей) на случай запрета `create_page`; (2) оркестратор/координатор создаёт страницу сам (`_schema="project_tickets"`), линкует её (план ↔ ревью) и в теле отмечает, что текст перенесён из read-only-отчёта ревьюера; (3) ID страницы — в финальном отчёте. Тип `review` требует `metadata.review_of` (UUID проверяемой страницы) и `metadata.reviewer` (непустая строка) — без них `create_page` отклоняет запрос. Молча пропущенный wiki-персист = нарушение Golden Rule.
8) 🔴 Check available workers before delegating.

## 🔴 Release gate — CI на коммите тега

GitHub Release публикуется только после зелёного CI **на том самом коммите, на который указывает релизный тег**. Правила обязательные. Релиз, опубликованный в обход, снимается **только по решению пользователя**; разбор инцидента через тикет при этом остаётся обязательным.

- 🔴 **Точный SHA, а не «где-то зелёное».** Все jobs workflow `CI` — без исключений и без сокращённого перечня, явно включая `Build (windows-latest)` — должны быть в статусе `success` на коммите, на который указывает тег. `success` на PR, на `main` или на любом другом SHA доказательством не является: тег мог быть поставлен на другой коммит, а повторный пуш/мерж меняет содержимое.
- 🔴 **Что считается подтверждённым падением Windows.** Классификация — job-level, по `docs/ci-triage.md`, а не по статусу run: run, часть job'ов которого отменена платформой, сам отчитывается `failure`. Инфраструктурная (capacity) отмена — это job с `conclusion: cancelled`, пустым `runner_name` и нулём выполненных шагов; тикет на неё как на дефект не заводится. Подтверждённое падение — job с `conclusion: failure` или `timed_out`, которому назначен runner и который дошёл до выполнения шагов; разбирается именно оно.
- 🔴 **Первое падение Windows разбирается, а не затирается ретраем.** Любое подтверждённое падение `Build (windows-latest)` обязано быть разобрано и зафиксировано в тикете: причина (ошибка из лога) + доказательство исправления (команда/тест и её результат). Текущий auto-retry (`.github/workflows/ci-retry.yml`) перезапускает только job'ы, отменённые в очереди по capacity, и реальные `failure`/`timed_out` не ретраит; успешный auto-retry обязательный разбор подтверждённого падения НЕ заменяет и отсутствие дефекта НЕ доказывает.
- 🔴 **Запись до публикации.** URL релевантного CI run (именно на релизном коммите) и результат проверки (список обязательных jobs с их статусами) фиксируются в релизном тикете ДО нажатия «Publish». Релиз без такой записи не публикуется.

## Commands (Go)

```bash
gofmt -l . && go vet ./...      # обязательно чисто перед коммитом
go test ./...                   # обязательно зелёное перед отметкой done
VER=$(git describe --tags --always 2>/dev/null | sed 's/^v//'); VER=${VER:-dev}
go build -ldflags "-X ticket/internal/cli.version=$VER" ./cmd/ticket    # локальный бинарник
GOOS=windows GOARCH=amd64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket.exe ./cmd/ticket   # для dser97
```

Правило: код/комментарии/идентификаторы/имена файлов — на английском; обсуждение и документация — на русском.

## Подпись коммитов (SSH-ключ ai-base через TPM2)

Коммиты подписываются **SSH-ключом** `~/.ssh/id_ed25519-ai-base` — тем же, что ходит на dser97; он зарегистрирован на GitHub как **Signing key**. GPG для коммитов больше не используется (решение пользователя 2026-10-26).

Глобальный конфиг (применён 2026-10-26, `~/.gitconfig`):
```
gpg.format = ssh
user.signingkey = <литеральная строка pubkey ai-base>   # не путь: приватный файл зашифрован и живёт в агенте
commit.gpgsign = true
gpg.ssh.allowedSignersFile = ~/.config/git/allowed_signers
```
Файл `~/.config/git/allowed_signers` содержит строку `<email> ssh-ed25519 AAAA… ai-base` — без него локальная проверка подписи не работает (`%G?` = N).

🔴 **Перед коммитом нужен TPM2-сокет:** в wezterm-сессии `SSH_AUTH_SOCK=/run/user/1000/wezterm/agent.7943` — там ключей НЕТ, коммит с `-S` падает с `Couldn't find key in agent?` (проверено). Агент выставляет сокет сам, в `~/.bashrc` ничего не прописано (решение пользователя: не надо):
```bash
export SSH_AUTH_SOCK=/run/ssh-agent-tpm2/agent.sock
```
Проверка подписи: `git commit --allow-empty -S -m probe && git log --format='%h %G?' -1` → ожидается `G`.

- Локальные GPG-настройки в `.git/config` (были `user.signingkey = 6AB4D0E6AA282FC9`) сняты — иначе они перебивают глобальные.
- GPG-ключ `6AB4D0E6AA282FC9` остаётся для шифрования и подписи файлов. Если понадобится GPG-подпись: `echo test | gpg --clearsign -u 6AB4D0E6AA282FC9 > /dev/null` — вводит пароль в кэш gpg-agent (`default-cache-ttl 14400` = 4 ч скользящего окна, `max-cache-ttl 999999` ≈ 11,57 сут). `-u` обязателен: в `~/.gnupg/gpg.conf` стоит чужой `default-key`.
- Мержи через `gh pr merge` подписывает сам GitHub своим ключом (`B5690EEEBB952194`), поэтому у таких коммитов локально `%G?` = `E` («проверить нельзя») — это нормально, не баг.
- Подробности по агенту/TPM2 — скилл `~/.agents/skills/ssh-agent-tpm2/SKILL.md`.

## Документы проекта (локальные, не в wiki)

| Файл | Что |
|------|-----|
| `AGENTS_ARCHITECTURE.md` | Архитектура (v1 draft) |
| `docs_api/component-catalog.md` | Component Catalog: окружения, toolchain, команды |
| `docs_api/tech-debt-open.md` / `docs_api/tech-debt-completed.md` | Реестры техдолга |
| `docs/security-accepted-risks.md` (+ `docs/ru/`) | Реестр принятых рисков CI (T-0207): workflow_run+actions:write, цепочка поставки инструментов, baseline zizmor |
| `docs_api/reference-init-tickets.md` | Устаревшая копия другой (bash) ревизии — НЕ эталон, сверяться не нужно (см. `AGENTS_ARCHITECTURE.md` §10) |
| `skills/ticket-cli/SKILL.md` | Скилл для агентов: работа с `ticket` CLI (копия — в `~/.agents/skills/ticket-cli/`) |

### 🔴 Правки скиллов — полная публикация в том же цикле

Скилл считается изменённым только тогда, когда в том же рабочем цикле обновлены, проверены, закоммичены и запушены ВСЕ его копии и связанные файлы.

- 🔴 **Тот же цикл, все файлы.** Правка `skills/<skill>/…` → обновить ВСЕ связанные файлы (repo + installed `~/.agents/skills/<skill>/`: `SKILL.md`, `reference.md`, плюс `AGENTS.md`/docs, где скилл упомянут) → проверить → закоммитить и запушить всё относящееся. Незакоммиченная или незапушенная правка скилла — работа НЕ сделана.
- 🔴 **Проверка перед коммитом.** `git diff` правок + валидность Markdown/frontmatter + сверка копий по назначению (`cmp` или body-diff). `gofmt`/`go vet`/`go test` — ТОЛЬКО если затронут Go-код; для docs-only правок они не требуются.
- 🔴 **Перед отчётом «DONE»:** tracked `git status --porcelain` пуст и `main == origin/main` (`git fetch`, сравнить `HEAD` с `origin/main`). Push невозможен (нет сети/прав/агента) — это явный blocker/PARTIAL с указанием причины, НЕ ложный DONE.
- 🔴 **Ignored/untracked файлы.** Связанный с правкой ignored/untracked файл нельзя оставлять локальным — иначе требование коммита невыполнимо. Публиковать его можно только при явном разрешении пользователя и точным `git add -f -- <path>` (никакого broad force-add). Без разрешения — blocker/PARTIAL с указанием причины, НЕ DONE. Абсолютный запрет force-add для `tickets/` остаётся в силе.
- 🔴 **Только свои файлы.** В коммит идут лишь файлы текущей задачи: посторонние/чужие uncommitted-изменения не добавлять (`git add <путь>`, не `-A`), не удалять и не revert-ить; обнаруженные чужие правки — сообщить пользователю.
- 🔴 **Никакого blind-copy `reference.md`.** Repo-копия — снимок ИСХОДНОГО шаблона help (`internal/cli/templates/usage.ru.txt`) и может опережать установленный бинарник; installed-копия — снимок реального `ticket help` бинарника из PATH. Сначала снять фактический вывод (`ticket help > /tmp/…`), сравнить body, и только затем обновлять installed строго по фактическому выводу (при совпадении body — не трогать). При обновлении installed-копии `verified_with_cli`, `version` и `modified` приводятся к фактической проверке, а File History дополняется новой записью.

## Тикеты (`tickets/`) — 🔴 ТОЛЬКО локальные файлы

Этот проект — сам система тикетов, и мы её же (dogfooding) ведём свои тикеты. CLI — `ticket` из PATH (`~/bin/ticket`, T-0057). 🔴 Симлинка `tickets/bin/ticket` больше НЕТ (проверено 2026-10-26: каталога `tickets/bin` нет) — в дереве проекта работает `ticket` из PATH; мёртвые ссылки на этот путь из Go-комментариев убраны в T-0096.

- 🔴 WikiDB `hot.tickets`, `ticket_create`, `list_tickets` для этого проекта **НЕ ИСПОЛЬЗОВАТЬ ВООБЩЕ** — тикеты только файлы в `tickets/`.
- 🔴 **Никогда не делать force-add того, что лежит в `tickets/`** — ни `git add -f tickets/<файл>`, ни `git add -f` с путём внутри `tickets/`, ни обходной `git add -f .` из корня: тикеты локальные, в git они не попадают (`.gitignore:2`, `/tickets/`). Если файл из `tickets/` оказался в `git status` как staged — это ошибка, снять с индекса `git reset -- tickets/<файл>` (файл на диске остаётся). Повод «закоммитить тикет для истории» не действует: история тикетов живёт только на машине. Регрессию ловит CI-гвард `.github/workflows/ci.yml` (job `lint`, шаг «Guard: tickets/ must not be tracked», T-0130).
- 🔴 **Локальные тикеты пишутся ПО-РУССКИ** (решение пользователя 2026-11-06): заголовок, Summary, Details, комментарии агента и журнал — русский язык. Английский остаётся для кода, комментариев в коде, идентификаторов и имён файлов (см. правило в «Commands (Go)»). Тикеты, созданные ранее по-английски, задним числом не переписываются — правило действует с 2026-11-06 вперёд.
- НА СТАРТЕ сессии: `ticket list` — если есть открытые тикеты, кратко напомни о них пользователю при первом же ответе.
- 🔴 Проверка новых релизов: только корневой оркестратор ОДИН раз в начале КОРНЕВОЙ сессии запускает `ticket update-check`; координаторы, планировщики, кодеры, ревьюеры, исполнители и любые субагенты живую проверку не запускают, не включают её в брифы и не перекладывают эту обязанность. Коды выхода 0/1/3 — ожидаемые, блокером не являются.
- Нашёл во время работы проблему, ошибку или что-то подозрительное — СРАЗУ создай тикет: `ticket new "<кратко>" -t BUG|OPS|TD|ENH -p low|normal|high -d "<подробности>"`. НЕ молчи, даже если удалось обойти/починить по месту. В подробностях указывай файл:строка. 🔴 Исключение: проблемы самой инфраструктуры WikiDB — тикет про них НЕ создавать (см. «WikiDB будет заморожен» выше), достаточно рабочей заметки в текущей задаче.
- Смена статуса — только через `ticket set <номер> <статус> "<комментарий>"` (инструмент сам переименует файл и допишет журнал тикета); вручную файлы не переименовывать.
- 🔴 Перед работой над тикетом агент обязан прочитать секцию «## Комментарии от пользователя» — там пользователь оставляет замечания; агент туда НЕ пишет. Свободная секция «## Комментарии» — для рабочих заметок агента (T-0032, T-0035; секции создаёт Go-версия — в новых тикетах они есть сразу, а bash-созданные получают недостающие при первом `set`/`archive` Go-бинарём).
- 🔴 **Гейт закрытия тикета:** верифицировал фикс (тесты/команда пройдены) → НЕМЕДЛЕННО `ticket set <номер> done "<комментарий с доказательством>"`. В комментарии — что именно проверено: команда + результат, файл:строка. Статус `done` без доказательства недопустим. Владелец смены статуса — тот, кто верифицировал фикс: воркер верифицирует и сам закрывает, координатор контролирует по чек-листу ниже. НЕ копить пачкой «потом проставлю».
- 🔴 **Чек-лист перед отчётом пользователю:** прогнать `ticket list` и сверить статусы с реальностью — всё, что реально исправлено и проверено, должно быть `done`. Расхождения устранить ДО отчёта, а не после напоминания.
- 🔴 Тесты/smoke команды `ticket` — ТОЛЬКО в песочнице (копия скрипта в /tmp, `TICKETS_DIR` или `-C /tmp/…` для Go-версии), НИКОГДА в боевом `tickets/` — инцидент T-0023 (мусорные тикеты, закрытие T-0001).
- Закрытых тикетов (`done`/`closed`) стало много — перенеси в архив: `ticket archive`. Архивные не теряются: `ticket list archive`, `show`/`set` работают и с ними, reopen возвращает в работу.
- В тикеты НЕ писать секреты (пароли, токены, ключи).
- Планы/ревью/решения по гейтам — по-прежнему страницы в wiki-схеме `project_tickets`.
- Скилл `skills/ticket-cli/SKILL.md` (поставляется в репозитории, также установлен в `~/.agents/skills/ticket-cli/`) обучает агента работе с ticket CLI — агентам СЛЕДУЕТ его загружать.

## Token Thrift (keep sessions cheap)

1. `list_tickets`: ALWAYS filter (status open/in_progress + projectName). NEVER dump the whole table (65KB incident 2026-08-27). (Для ЭТОГО проекта hot.tickets запрещён вовсе — правило относится к кросс-проектным запросам.)
2. Spot-verification: targeted SQL probes (`substring(content…)`, counts, `content_hash`) over full `get_page`; `search_pages` small limit.
3. Subagent briefs MUST demand: concise final message — recommended ≤150 words (up to ~350 for multi-part tasks; recommendation, NOT a hard cap) + wiki page/ticket IDs; ALL detail persisted to wiki BEFORE returning. Same bar for handoffs.
4. Startup: gates + role + limits only; everything else on-demand.

## Golden Rule

If it's not in the wiki, it didn't happen.
