# Реестр принятых рисков безопасности

Реестр принятых рисков безопасности поверхности CI/CD, зеркало
[docs/security-accepted-risks.md](../security-accepted-risks.md). Каждая запись
фиксирует риск, сценарий атаки, контроль, делающий риск приемлемым, и точное
подавление в
[.github/zizmor.yml](../../.github/zizmor.yml) (узкие ignore `файл:строка:колонка`;
ни одно правило и ни один класс серьёзности не отключаются). Сама аудит-джоба
описана в `.github/workflows/ci.yml` (job `security`); процедура обновления
и пересмотра baseline — в [docs/ru/ci-triage.md](ci-triage.md).

## (a) ci-retry.yml: workflow_run + actions:write (T-0202)

- **Риск:** `.github/workflows/ci-retry.yml` держит `actions: write` и
  работает по `workflow_run` — privileged-джоба рядом с PR.
- **Сценарий:** вредоносный PR не может исполнить код в этом workflow (в нём
  нет checkout и PR-контролируемых шагов; только `gh api` против
  доверенных данных API). Реалистичный сценарий дрейфа — будущая правка,
  добавляющая PR-контролируемое исполнение или работу с артефактами в
  privileged-контексте, либо изменение метаданных `workflow_run`, после
  которого rerun затрагивает больше, чем задумано.
- **Контроль:** изменение оформлено как одношаговый fail-closed дизайн
  (задокументировано в T-0202); persona `auditor` zizmor показывает любой
  дрейф этого privileged workflow (excessive-permissions High/High не виден
  под дефолтной persona `regular`); эта запись реестра делает принятие
  явным. Сам файл
  вне скоупа PR T-0207 и не должен правиться ради глушения findings.
- **Подавление (только этот finding):**
  `dangerous-triggers ci-retry.yml:14:3`,
  `excessive-permissions ci-retry.yml:31:3`,
  `undocumented-permissions ci-retry.yml:31:3,50:7` (permissions явные;
  дублирование осознанно),
  `concurrency-limits ci-retry.yml:13:1` (ci-retry стартует на каждый CI
  run; concurrency-группа сломала бы функцию ретрая).
- **Ссылки:** T-0202, T-0207, консультация `b431407f-7242-4963-9ea3-ee00ded6d0bb`.

## (b) Цепочка поставки CI-инструментов

- **Риск:** lint-джоба скачивает инструменты по мутабельным тегам —
  `go install staticcheck@v0.8.1`, `go install gosec@v2.30.0`,
  `npx --yes markdownlint-cli2@0.23.3`. Пере-тегированный или скомпрометированный
  релиз исполнился бы в CI без локальной проверки целостности.
- **Сценарий:** компрометация upstream-аккаунта или вредоносный релиз под
  существующим тегом; отравленный бинарь исполняется на раннере с (read-only)
  токеном джобы.
- **Контроль:** принято как инкремент, а не новый класс риска (advisory §2):
  эти инструменты уже работали в CI до появления аудита. Проверяемая
  закрепка сегодня только у zizmor: release-ассет v1.32.0 + SHA-256
  `bb110cc765eca9cd7efb2e284a956be5974c60027b0970f16397d86208a9957b`
  (равен digest из GitHub API на момент закрепки) + offline-проверка
  Sigstore-аттестации (SLSA provenance/v1) по вендоренным
  `.github/zizmor/bundle.json` /
  `.github/zizmor/trusted_root.jsonl`. Образ ghcr.io отвергнут: аттестации
  на него нет. Триггеры ужесточения: любые новые секреты, self-hosted
  runner, `pull_request_target` или новый privileged workflow в CI — тогда
  вариант A advisory не отменяется, вместо этого ужесточается изоляция
  audit-джобы (advisory §5).

## (c) Baseline-findings, принятые в ci.yml

- **Риск:** `cache-poisoning` (кэши setup-go/setup-node) и отсутствие
  workflow-level группы `concurrency`.
- **Сценарий:** для отравления кэша fork-PR должен писать в кэш базовой
  ветки — не может (read-only токен в PR); загружаемые артефакты — собственные
  исходники репо. Concurrency-группа отменила бы pending-джобы с пустым
  `runner_name` и сломала бы триаж capacity-отмен (T-0196/T-0203).
- **Контроль:** `cache:false` заставил бы пересобирать toolchain каждый
  прогон и отвергнут (confidence finding Low); `fail-fast: false`
  сохранён осознанно.
- **Ограничение дизайна:** на `pull_request` аудит-джоба делает checkout
  содержимого PR, поэтому PR может править аудируемые workflows/конфиг,
  чтобы пройти собственный аудит; гарантия fail-closed держится на push в
  main (доверенная base). Изоляция джобы (без секретов/кэшей/артефактов)
  компенсирует.
- **Подавление (только этот finding):**
  `cache-poisoning ci.yml:46:9,114:9,151:9`,
  `concurrency-limits ci.yml:5:1`.

## (d) ci-metrics.yml: schedule + actions:read (T-0209)

- **Риск:** `.github/workflows/ci-metrics.yml` по таймеру выполняет код репозитория (`scripts/ci-metrics.sh`, ветка по умолчанию) и держит `actions: read`, чтобы читать данные прогонов и джоб.
- **Сценарий:** будущая правка добавит стороннее действие или PR-контролируемое исполнение в эту scheduled-джобу, либо read-скоуп тихо расширят до `actions: write` в workflow, который никто не ревьюит на уровне PR. Сами данные агрегированные: секретов в отчёте нет, токен никогда не печатается.
- **Контроль:** явные permissions на уровне workflow (`contents: read`, `actions: read`, ничего больше); единственное действие — SHA-закреплённый `actions/checkout` с `persist-credentials: false`; ни кэша, ни артефактов, ни записей в `GITHUB_ENV`/`GITHUB_OUTPUT`, ни интерполяции `${{ }}` внутри `run:`; скрипт fail-closed (любой не-OK вердикт = exit 1 = красный прогон). Дрейф ловит закреплённая persona `auditor` zizmor, сейчас через локальный повторный аудит: CI-джоба `security` аудирует только `ci.yml` + `ci-retry.yml` (ci.yml:263-267), а T-0212 отслеживает добавление нового файла.
- **Ограничение дизайна:** `GITHUB_TOKEN` не может читать аудит-лог организации (нужны admin/org-скоупы) — эта проверка остаётся ручной и выполняется владельцем; workflow её не автоматизирует.
- **Подавление:** нет. Baseline-запись для этого файла не добавляется; findings предотвращаются конструкцией. Ослабление `.github/zizmor.yml` вне скоупа 5-файлового изменения.
- **Ссылки:** T-0209, T-0212, T-0207, T-0211.

## Журнал аудита

| Дата | zizmor | Commit репо | SHA-256 вендоренного bundle | SHA-256 вендоренного root | Результат аудита | Следующий пересмотр |
|------|--------|-------------|-----------------------------|---------------------------|------------------|---------------------|
| 2026-11-19 | v1.32.0 | 6ad2f0b (PR #36; base b56993f) | `5cf002bf…b97b25ef` | `65ca537f…2eb2b9c` | auditor: 0 видимых findings, 9 ignored (baseline выше); без конфига: 9 findings | 2027-02-17 |
| 2026-11-20 | v1.32.0 | 9289645 (base; tip на момент аудита) + изменения worktree t-0209 | `5cf002bf…b97b25ef` | `65ca537f…2eb2b9c` | auditor: 0 видимых findings, 9 ignored (baseline ci.yml+ci-retry.yml; аудит 3 workflow); --no-ignores: 9 findings (4 low, 5 high), ни одного в ci-metrics.yml | 2027-02-18 |

Политика: пересмотр на каждый бамп версии/digest zizmor, каждую правку
workflow, каждый новый privileged-контур CI либо раз в 90 дней — что раньше.
Процедура: [docs/ru/ci-triage.md](ci-triage.md) §«Security audit: обновление
zizmor и пересмотр baseline».
