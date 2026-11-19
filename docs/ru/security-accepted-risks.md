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

## Журнал аудита

| Дата | zizmor | Commit репо | SHA-256 вендоренного bundle | SHA-256 вендоренного root | Результат аудита | Следующий пересмотр |
|------|--------|-------------|-----------------------------|---------------------------|------------------|---------------------|
| 2026-11-19 | v1.32.0 | 6ad2f0b (PR #36; base b56993f) | `5cf002bf…b97b25ef` | `65ca537f…2eb2b9c` | auditor: 0 видимых findings, 9 ignored (baseline выше); без конфига: 9 findings | 2027-02-17 |

Политика: пересмотр на каждый бамп версии/digest zizmor, каждую правку
workflow, каждый новый privileged-контур CI либо раз в 90 дней — что раньше.
Процедура: [docs/ru/ci-triage.md](ci-triage.md) §«Security audit: обновление
zizmor и пересмотр baseline».
