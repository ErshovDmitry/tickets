# Релизы

Полный runbook публикации подписанного релиза: создание аннотированного тега с GPG-подписью, сборка шести платформенных бинарников, подпись контрольных сумм, публикация через `gh` и проверка опубликованных ассетов. Цели сборки и флаги — в [development.md](development.md); доступ к хостам и гейт деплоя — в локальном `DEPLOY.md` на ПК разработки (неотслеживаемый агентский файл, не часть репозитория). Здесь описан весь релизный путь.

Команды ниже используют `v2.0.1` как пример версии — подставляйте версию, которую выпускаете. Релизный GPG-ключ — `6AB4D0E6AA282FC9`, полный отпечаток `0CAF1003F44DD0E4990F4EFF6AB4D0E6AA282FC9`.

## Предварительные условия

- Зафиксирован финальный коммит `origin/main` (PR changelog смержен), и CI на нём зелёный: Lint, Build (ubuntu/macos/windows), Security audit.
- `gofmt -l .` пуст, `go vet ./...` и `go test ./...` чистые (гейт деплоя из `DEPLOY.md`).
- Релизный GPG-ключ `6AB4D0E6AA282FC9` есть в локальном keyring (`gpg --list-keys 6AB4D0E6AA282FC9`).
- Коммиты остаются подписанными SSH-ключом ai-base через TPM2-backed ssh-agent, как настроено глобально. **Не** меняйте глобальный git config ради релиза: подпись тега использует per-command OpenPGP-переопределения (ниже), потому что глобальный `gpg.format` — `ssh` для коммитов.

## Сборка из обычного клона

Релизные артефакты собираются только из **обычного `git clone`**, никогда из связанного worktree:

```bash
git clone https://github.com/ErshovDmitry/tickets.git
cd tickets
git checkout main
git rev-parse HEAD   # финальный релизный коммит; запишите его
```

**Не** собирайте релизные артефакты в связанном worktree (`git worktree add`). В Go 1.26 бинарник, собранный в linked worktree, *может* не содержать build-настроек `vcs.revision` и `vcs.modified`. Это не утверждение, что любая такая сборка обязательно их теряет — конкретный checkout может их и записать, — но полагаться на это нельзя, поэтому linked worktree не гарантирует провенанс. Поддерживаемый путь релизной сборки — обычный клон.

## Создание и проверка подписанного тега

Создайте аннотированный тег, подписанный релизным GPG-ключом. Бэкенд подписи и ключ выбираются per-command, поэтому глобальная SSH-настройка подписи коммитов остаётся нетронутой:

```bash
git -c gpg.format=openpgp -c user.signingkey=6AB4D0E6AA282FC9 \
  tag -s -u 6AB4D0E6AA282FC9 -m v2.0.1 v2.0.1 <commit>
```

`<commit>` — финальный релизный коммит, записанный выше. `-s`/`-u` создают криптографически подписанный аннотированный объект тега; `-u` выбирает ключ подписи, а `gpg.format` — бэкенд (здесь OpenPGP, переопределяя глобальный `ssh`).

Проверьте тег теми же OpenPGP-переопределениями:

```bash
git -c gpg.format=openpgp -c user.signingkey=6AB4D0E6AA282FC9 tag -v v2.0.1
```

Ожидается: действительная подпись от `0CAF1003F44DD0E4990F4EFF6AB4D0E6AA282FC9`, и объект тега указывает на финальный релизный коммит. Без переопределений `git tag -v` использует глобальный формат `ssh` и не может проверить OpenPGP-тег.

Переключитесь на тег для сборки и возьмите версию из него:

```bash
git checkout v2.0.1
VER=$(git describe --tags --exact-match | sed 's/^v//')
go version                 # Go >= 1.26, см. development.md
```

## Сборка всех релизных ассетов

Соберите все цели с `CGO_ENABLED=0`, сохраняя стабильные имена ассетов (`ticket-<os>-<arch>`, `.exe` для Windows):

```bash
for os in linux darwin windows; do
  for arch in amd64 arm64; do
    ext=""
    [ "$os" = windows ] && ext=".exe"
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
      go build -ldflags "-X ticket/internal/cli.version=$VER" \
      -o "dist/ticket-$os-$arch$ext" ./cmd/ticket
  done
done
```

В результате получаются шесть бинарников: `ticket-linux-amd64`, `ticket-linux-arm64`, `ticket-darwin-amd64`, `ticket-darwin-arm64`, `ticket-windows-amd64.exe`, `ticket-windows-arm64.exe`. Релизный ассет windows/amd64 — `ticket-windows-amd64.exe`, а **не** dev-`dist/ticket.exe` из таблицы кросс-компиляции в [development.md](development.md).

Рядом с ними создайте файл контрольных сумм и его отделённую подпись:

```bash
cd dist
sha256sum ticket-linux-amd64 ticket-linux-arm64 \
  ticket-darwin-amd64 ticket-darwin-arm64 \
  ticket-windows-amd64.exe ticket-windows-arm64.exe > SHA256SUMS.txt
gpg --armor --detach-sign -u 6AB4D0E6AA282FC9 SHA256SUMS.txt
```

Подпись должна быть **отделённой ASCII-armored** (`SHA256SUMS.txt.asc`, начинается с `-----BEGIN PGP SIGNATURE-----`) — не clearsign, который переписал бы сам файл сумм. `-u 6AB4D0E6AA282FC9` фиксирует ключ подписи. Таким образом, релиз содержит восемь ассетов: шесть платформенных бинарников, `SHA256SUMS.txt` и `SHA256SUMS.txt.asc`.

## Локальная проверка до публикации

Посмотрите встроенную build-информацию в каждом из шести бинарников:

```bash
for f in dist/ticket-*; do go version -m "$f" | grep -E 'vcs\.|cli.version'; done
```

Каждый релизный бинарник обязан содержать:

- строку `build vcs.revision=<commit>`, где commit совпадает с `git rev-parse HEAD` тегированного checkout;
- `build vcs.modified=false`;
- значение `-ldflags`, содержащее `ticket/internal/cli.version=<версия>`.

Модуль без суффикса `/vN` Go записывает как псевдоверсию (например, `v1.2.7-0.<timestamp>-<commit>`), а не как имя тега; авторитетная версия — это тег плюс строка `-ldflags`, а не эта псевдоверсия.

Затем локально проверьте суммы, подпись и версию:

```bash
cd dist
sha256sum -c SHA256SUMS.txt
gpg --verify SHA256SUMS.txt.asc SHA256SUMS.txt
./ticket-linux-amd64 --version   # ticket version 2.0.1
```

Если хотя бы один пункт buildinfo отсутствует, артефакт собран не из чистого checkout на теге — пересоберите его из обычного клона и не публикуйте.

## Push тега

Запушьте подписанный тег **до** создания релиза:

```bash
git push origin v2.0.1
```

`gh release create --verify-tag` прерывается, если тега ещё нет в удалённом репозитории, поэтому push — обязательное предварительное условие.

Push по HTTPS аутентифицируется через credential-helper `gh` (`gh auth git-credential`), уже настроенный на ПК разработки; ни токен, ни пароль не фигурируют в команде и в этом runbook. Если на машине без helper push запрашивает учётные данные, один раз выполните `gh auth setup-git` (хост должен быть уже аутентифицирован через `gh auth login`) и повторите push.

## Публикация релиза

Сначала подготовьте файл notes. Он должен содержать английский раздел changelog `[2.0.1]` с датой релиза и только реально вошедшими изменениями (из `git log v2.0.0..HEAD` и diffs, а не из пустого Unreleased), полный отпечаток ключа `0CAF1003F44DD0E4990F4EFF6AB4D0E6AA282FC9` и команды проверки (`gpg --verify SHA256SUMS.txt.asc SHA256SUMS.txt`, `sha256sum -c SHA256SUMS.txt`) — образец: раздел Assets в опубликованных notes v2.0.0.

Публикуйте с `--verify-tag` и **без** `--target`:

```bash
gh release create v2.0.1 --verify-tag --notes-file notes-v2.0.1.md \
  dist/ticket-linux-amd64 dist/ticket-linux-arm64 \
  dist/ticket-darwin-amd64 dist/ticket-darwin-arm64 \
  dist/ticket-windows-amd64.exe dist/ticket-windows-arm64.exe \
  dist/SHA256SUMS.txt dist/SHA256SUMS.txt.asc
```

`--verify-tag` прерывается, если тег не существует на remote, — это гарантирует, что релиз опирается на запушенный аннотированный тег с GPG-подписью. Без него `gh` при отсутствии тега молча создал бы lightweight-тег из default-ветки, потеряв подпись; `--target` управляет только этим автосозданием тега и в данном потоке не нужен.

Проверьте загруженный набор по именам — ровно восемь ассетов:

```bash
gh release view v2.0.1 --repo ErshovDmitry/tickets --json assets --jq '.assets[].name'
```

## Проверка опубликованного релиза (свежее скачивание)

Скачайте ассеты заново из опубликованного релиза в чистый каталог (не из каталога сборки) и повторите все проверки:

```bash
gh release download v2.0.1 --repo ErshovDmitry/tickets --dir release-check
cd release-check
sha256sum -c SHA256SUMS.txt
gpg --verify SHA256SUMS.txt.asc SHA256SUMS.txt
go version -m ticket-linux-amd64 | grep -E 'vcs\.|cli.version'
chmod +x ticket-linux-amd64      # скачанные ассеты теряют executable-бит
./ticket-linux-amd64 --version   # ticket version 2.0.1
cd ..                            # возврат в корень репозитория для Windows-smoke ниже
```

`sha256sum -c` должен показать `OK` для каждого файла, а `gpg` — действительную подпись от `0CAF1003F44DD0E4990F4EFF6AB4D0E6AA282FC9`. Buildinfo снова должен показывать `vcs.revision`, равный коммиту тега, и `vcs.modified=false`.

При сверке с предыдущим релизом (v2.0.0) эталон — `gh release view` / свежее `gh release download`, а не старый локальный каталог `dist/`. Зафиксируйте фактические результаты этого релиза в его evidence-заметке; никогда не переносите контрольные суммы и результаты проверки из более раннего релиза в более поздний — дайджесты у каждой сборки свои.

## Версия и smoke на Windows (dser97)

SSH-доступ к dser97 уже настроен: ключ ai-base живёт в TPM2-backed ssh-agent на ПК разработки, поэтому `ssh dser97.home` / `scp` работают как есть — ключ не запрашивать (см. `DEPLOY.md`).

Из корня репозитория (блок свежего скачивания выше возвращает туда) скопируйте релизный бинарник и smoke-скрипт в домашний каталог SSH, затем проверьте версию и запустите smoke оттуда:

```bash
scp dist/ticket-windows-amd64.exe scripts/smoke-windows.ps1 dser97.home:
```

```powershell
.\ticket-windows-amd64.exe --version   # ticket version 2.0.1
powershell -ExecutionPolicy Bypass -File .\smoke-windows.ps1 -TicketExe .\ticket-windows-amd64.exe
```

Smoke-скрипт работает целиком в temp-песочнице (репозиторий и пользовательские данные не затрагиваются) и покрывает восемь шагов: `new`, `list`, `show`, `set`, глобальный флаг `-C`/`--tickets-dir` из чужой CWD, переопределение `TICKETS_DIR`, пять параллельных `new` и `init`. Exit code 0 — все шаги пройдены, 1 — хотя бы один упал. Для читаемого UTF-8-вывода через SSH запускайте команды через `powershell -NoProfile -Command "[Console]::OutputEncoding=[Text.Encoding]::UTF8; ..."`.

## Неизменяемость релиза

После публикации тег и ассеты неизменяемы: никакого re-tag, re-upload и замены ассетов. Дефект, найденный после публикации, исправляется следующим patch-релизом, а не правкой опубликованного.
