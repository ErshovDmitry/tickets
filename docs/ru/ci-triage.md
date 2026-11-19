# CI-триаж

Эта процедура разделяет два совершенно разных случая за красным прогоном GitHub Actions: capacity-отмену (платформа забрала job до его запуска) и настоящее падение теста. Различие определяется по данным job на уровне attempt, а не по статусу прогона, потому что прогон с capacity-отменёнными job сам сообщает `conclusion: failure`. Далее процедура говорит, что делать: перезапустить упавшие job или чинить код.

## Признаки capacity-отмены

Job отменён платформой, а не тестовым набором, когда присутствуют все три job-level признака:

- `conclusion` равен `cancelled`;
- `runner_name` пуст — раннер не назначался вообще;
- `len(steps)` равен `0` — job не начал ни одного шага.

Время в очереди (около 902 с по натурному замеру T-0196) — только справочный ориентир, а не обязательный признак: в фикстурных данных T-0196 у infrastructure-cancel `queue = 0`.

Поскольку сам прогон заканчивается с `conclusion: failure`, фильтр по `runs?status=cancelled` бесполезен — прогон изначально не сообщает `cancelled`.

## Отличие от падения теста

Настоящее падение теста отличается по всем осям:

- `conclusion` равен `failure`;
- `runner_name` непуст — раннер был назначен, job стартовал;
- шаги присутствуют, а в логе упавшего шага видно упавшую проверку или упавшую сборку.

Действие здесь — чинить код. Перезапускать job не нужно: то же самое падение повторится.

## Исторические fail-fast-каскады

15 отменённых job исторического окна распадаются на две группы: 6 capacity-отмен (раннер не назначался) и 9 fail-fast-соседей (job отменялись из-за падения соседнего job той же матрицы, пока действовал `fail-fast: true`, до PR #30).

После `fail-fast: false` (ci.yml, PR #30) отмены соседей исчезли. С тех пор отменённый job подозревается на capacity только при наличии job-level признаков (пустой `runner_name` и ноль шагов).

## Что делать

Перезапускать только упавшие job затронутого прогона:

```bash
gh run rerun <run-id> --failed
```

Штатным путём в будущем станет ci-retry.yml stage 2 (T-0202); сегодня существует только stage 1, и он работает в режиме dry run.

## Еженедельная процедура

Раз в неделю запускаем скрипт метрик:

```bash
bash scripts/ci-metrics.sh --days 30 --cache ~/.cache/ticket-ci-metrics
```

Вывод состоит из следующих блоков:

- перцентили очереди `p50`/`p95`/`max` по каждой операционной системе за последние 30 дней;
- отдельный блок macOS за последние 14 дней (`p95` и число job) — очередь macOS самая медленная, и с ней связан порог тревоги;
- сводка отменённых job: infrastructure-отмены (`runner_name` пуст, ноль шагов) против прочих отмен;
- доля перезапусков: PR-прогоны с rerun в процентах от всех PR-прогонов окна;
- вердикт: `OK`, `ALARM`, `INCOMPLETE`, `NO_DATA` или `ERROR`, у каждого свой маркер.

`INCOMPLETE` означает неполные данные (пагинация, см. T-0204), `NO_DATA` — в окне не было ни одного job.

Коды возврата: `0` — только `OK`; `1` — `ALARM`, `INCOMPLETE`, `NO_DATA` или `ERROR`; `2` — некорректные аргументы.

При `ALARM`: открыть тикет и рассмотреть rerun-политику и self-hosted runner.

Механика данных: данные job берутся только по attempt, через `/actions/runs/<id>/attempts/<n>/jobs`; время в очереди — `started_at − created_at`, job с `started_at < created_at` (переиспользованные) исключаются; сырой кэш лежит вне репозитория.

## Пороги тревоги

Пороги — решение советника по тикету T-0203, дословно:

> ≥3 infrastructure-cancel ИЛИ >10 % PR-прогонов с rerun за 30 дней ИЛИ p95 macOS > 15 мин при ≥30 job за 14 дней

Скрипт хранит их как именованные константы: CANCEL_MIN=3, RERUN_PCT=10, MACOS_P95_MAX_SEC=900, MACOS_MIN_JOBS=30, MACOS_DAYS=14. Каждую константу можно переопределить флагом.

## Планирование и честность про cron

Scheduled workflow возможен только после подключения zizmor (T-0207 смержен; cron блокирован T-0208). До тех пор еженедельный запуск ручной, и он выполняется по этой процедуре:

```bash
bash scripts/ci-metrics.sh --days 30 --cache ~/.cache/ticket-ci-metrics
```

## Оговорка про stage 2

После включения ci-retry stage 2 метрика `rerun%` начнёт считать и автоматические rerun'ы, а не только ручные. Скрипт печатает эту оговорку в своём выводе.

## Security audit: обновление zizmor и пересмотр baseline

Джоба `security` в `.github/workflows/ci.yml` аудирует оба workflow
закреплённым zizmor. Красный прогон Security audit означает НОВЫЙ finding
(всё из задокументированного baseline игнорируется и остаётся зелёным).
Порядок триажа:

1. **Решение о версии.** Прочитать changelog/release notes upstream для
   новой версии zizmor. Бамп — отдельный осознанный commit, никогда не в
   связке с другими изменениями.
2. **Ассет.** Скачать release-ассет по точному URL и сверить `sha256sum` с
   полем `digest` из `gh api repos/zizmorcore/zizmor/releases/latest`
   (или со страницы релиза).
3. **Аттестация.** Убедиться, что релиз несёт Sigstore-аттестацию от
   workflow `release-binaries.yml`:
   `gh attestation verify <asset> --repo zizmorcore/zizmor`.
4. **Вендоринг (offline-путь, LOCK).** Обновить вендоренные файлы
   `.github/zizmor/bundle.json` и `.github/zizmor/trusted_root.jsonl`
   под локом wiki. Bundle выбирать ПО ПРЕДИКАТУ — запись с
   `SLSA provenance/v1` (например, `gh api
   repos/zizmorcore/zizmor/attestations/<digest>`), не по индексу массива;
   trusted root — через `gh attestation trusted-root`. Затем проверка
   OFFLINE с изоляцией креденшелов (пустой конфиг-каталог, токены сняты):
   `env -u GH_TOKEN -u GITHUB_TOKEN GH_CONFIG_DIR=<пустой>
   gh attestation verify <asset> --bundle .github/zizmor/bundle.json
   --custom-trusted-root .github/zizmor/trusted_root.jsonl
   --repo zizmorcore/zizmor
   --signer-workflow zizmorcore/zizmor/.github/workflows/release-binaries.yml
   --source-ref refs/tags/vX.Y.Z`.
   Ожидание: exit 0 для подлинного ассета, 1 для подпорченного, 1 для
   bundle от другого подписанта — проверить все три.
5. **ДО.** Прогнать аудит на обоих workflow с ТЕКУЩИМ конфигом и зафиксировать
   числа (`ZIZMOR_CONFIG=.github/zizmor.yml ./zizmor --persona auditor
   --offline .github/workflows/ci.yml .github/workflows/ci-retry.yml`, плюс
   прогон без конфига).
6. **Закрепка одним commit.** Обновить URL+SHA-256 ассета в `ci.yml`, хэши
   вендоренных файлов в `ci.yml`, ignore-записи (позиции ПЕРЕИЗМЕРИТЬ, не
   копировать) в `.github/zizmor.yml` и строку журнала аудита в
   `docs/security-accepted-risks.md` (+ RU-зеркало) — ОДНИМ commit.
7. **ПОСЛЕ.** Повторный прогон аудита: ожидается «No findings to report» со
   счётчиком ignored, равным размеру baseline, exit 0 и зелёная джоба
   Security audit в CI. Числа ДО/ПОСЛЕ — в отчёт.

Реестр принятых рисков (`docs/ru/security-accepted-risks.md`) определяет,
какие findings могут оставаться в baseline и когда следующий пересмотр
(любой бамп версии/digest, любая правка workflow, любой новый
privileged-контур CI либо 90 дней — что раньше).

## Заметка о локальном доме

Отслеживаемым каноническим домом этой процедуры является эта пара документов: `docs/ci-triage.md` (EN) и `docs/ru/ci-triage.md` (RU). Волна добавляет в локальный, игнорируемый git `AGENTS_ARCHITECTURE.md` (боевое дерево) раздел-указатель со ссылкой на `docs/ci-triage.md` — полный текст там не дублируется.
