[English](README.md) | **Русский**

# ticket

Кроссплатформенная (Windows / Linux / macOS) CLI-система тикетов на Go.

## Что это

- Тикеты — обычные Markdown-файлы `T-NNNN-<status>.md` в каталоге `tickets/` проекта; единственный источник правды — файлы: читаются человеком без инструментов (`cat`, редактор, `git diff`), база данных не нужна.
- Один статический бинарник `ticket` (`ticket.exe` на Windows), собирается с `CGO_ENABLED=0`, ноль runtime-зависимостей.
- Кроссплатформенный аналог bash-версии [init-tickets.md](https://gitlab.com/ai-dmitry/promts/-/blob/main/init-tickets.md).

## 🎬 Видео

[![Смотреть на YouTube](https://img.youtube.com/vi/UW6O2zhEPwc/hqdefault.jpg)](https://youtu.be/UW6O2zhEPwc)

- [YouTube](https://youtu.be/UW6O2zhEPwc)
- [Дзен](https://dzen.ru/video/watch/6a9a61d4b6339503daf3830e)
- [Rutube](https://rutube.ru/video/29ab80e52b2d3c7afaef1eae0d0a0565/)

## Быстрый старт

Из корня проекта:

```bash
ticket init                                             # создать tickets/ (идемпотентно)
ticket new "Поломка экспорта" -t BUG -p high -d "Падает на больших файлах"
ticket list                                             # активные тикеты (open + wip)
ticket show 7                                           # показать тикет (ищет и в архиве)
ticket set 7 done "починено, тесты зелёные"             # сменить статус, файл переименуется сам
```

## Документация

- [Установка](docs/ru/installation.md) — бинарник в `PATH`, резолюция каталога тикетов (`-C`, `$TICKETS_DIR`, восходящий поиск), один бинарь на все проекты, примечание о миграции.
- [Использование](docs/ru/usage.md) — полная справка по командам, формат файла тикета.
- [Конфигурация](docs/ru/configuration.md) — переменные окружения `TICKETS_DIR`, `TICKET_WHO`, `TICKET_LANG`.
- [Сценарии](docs/ru/scenarios.md) — типовые рабочие сценарии: интеграция с агентами (блок `AGENTS.md`), скилл для агентов, архив, кросс-проектные тикеты.
- [Разработка](docs/ru/development.md) — сборка из исходников, кросс-компиляция, тесты, Windows-smoke.

## Ссылки

- bash-оригинал: [init-tickets.md](https://gitlab.com/ai-dmitry/promts/-/blob/main/init-tickets.md)

## Статус

**v1 done.**

## Что дальше

Есть мысли о дальнейшем развитии ticket, но они будут опубликованы позже, в комплексном виде. Когда будет что показать — здесь появится ссылка.
