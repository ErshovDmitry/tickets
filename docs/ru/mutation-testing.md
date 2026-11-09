# Mutation-тестирование

## Установка

Пин версии — `scripts/mutation.sh`. Ставить gremlins нужно в тот каталог, откуда скрипт его запускает; запускаются команды из корня репозитория (скрипт сам переходит туда и ищет бинарь относительно корня). `GOBIN` обязан быть абсолютным путём, иначе `go install` отказывается: `cannot install, GOBIN must be an absolute path`.

```bash
GOBIN="$PWD/.cache/gobin" go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0
GOBIN="$GOBIN" go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0
```

Первая — если `GOBIN` не задан **или пуст**: скрипт читает `./.cache/gobin/gremlins`. Вторая — если `GOBIN` задан **и непуст**: скрипт читает `$GOBIN/gremlins`, поэтому ставить надо именно туда (в этом случае `GOBIN` тоже должен быть абсолютным). `PATH` и `$(go env GOPATH)/bin` не ищутся.

Проверено: Go go1.26.10, раннер v0.6.0.

Почему gremlins: `go-mutesting` паникует в `go/types` на go1.26.10, `ooze` v0.2.0 — ложные killed (`go:embed`+symlink, T-0136).

## ⚠️ Предупреждение

Мутации идут В РАБОЧЕМ дереве. Грязное дерево и параллельный запуск запрещены: скрипт откажется (лок `./.cache/mutation.lock`).

## Запуск

```bash
./scripts/mutation.sh
```

Коды: 0 — успех, 1 — ошибка, 2 — неверные аргументы; INT/TERM — 130/143. Дефолты: скоуп `internal/lock`, `--workers 2`, `--timeout-coefficient 20`.

## Сводка

После прогона печатается блок `== Mutation summary ==` (`scripts/mutation.sh:250-260`); все числа берутся из JSON-отчёта gremlins.

- `Total` = `Killed + Lived`. Это знаменатель Efficacy (`Killed / Total`), а не «все мутанты».
- `Not covered`, `Timed out` и `Not viable` печатаются отдельными строками и в `Total` НЕ входят.
- Сумма учтённых скриптом статусов: `Total + Not covered + Timed out + Not viable`. `SKIPPED` и `RUNNABLE`, которые gremlins тоже выдаёт, скрипт не считает.
- `Timed out` — не отдельное поле JSON: скрипт считает вхождения статуса `"TIMED OUT"` в `files[].mutations[]` (статусы объявлены в gremlins `internal/report/report.go:106-123`).
- Скриптовый `Total` ≠ `mutants_total` из JSON: `mutants_total = lived + killed + not_viable` (`internal/report/report.go:178`).
