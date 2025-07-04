# GURLS-Bot

Telegram бот для управления короткими ссылками GURLS.

## Описание

GURLS-Bot предоставляет удобный интерфейс Telegram для создания, управления и получения статистики коротких ссылок. Общается с Backend через gRPC API.

## Команды бота

- `/start` - Главное меню с кнопками управления
- `/shorten <url> [опции]` - Создание короткой ссылки
  - `title="Название"` - Пользовательский заголовок
  - `expires_in=1h30m` - Время истечения (30m, 2h, 7d)
  - `alias=custom` - Пользовательский алиас
- `/stats <alias>` - Статистика по ссылке
- `/delete <alias>` - Удаление ссылки
- `/my_links` - Список всех ссылок пользователя

## Функциональность

- Создание ссылок с автогенерацией или пользовательскими алиасами
- Установка времени истечения ссылок
- Просмотр детальной статистики кликов
- Управление ссылками через удобные inline кнопки
- Обработка состояний пользователя для интерактивного создания ссылок

## Запуск

### Локальная разработка

```bash
# Установка зависимостей
go mod tidy

# Запуск бота
go run ./cmd/bot

# Или сборка и запуск
go build -o bin/bot ./cmd/bot
./bin/bot
```

### Конфигурация

Сервис использует файл `config/local.yml` или переменные окружения:

- `TELEGRAM_TOKEN` - токен Telegram бота (обязательно)
- `GRPC_BACKEND_ADDRESS` - адрес gRPC Backend сервиса (по умолчанию: localhost:50051)
- `BASE_URL` - базовый URL для формирования коротких ссылок
- `ENV` - окружение (local/dev/production)

### Получение токена бота

1. Перейдите к [@BotFather](https://t.me/BotFather) в Telegram
2. Создайте нового бота командой `/newbot`
3. Следуйте инструкциям для получения токена
4. Добавьте токен в файл `.env` или переменную окружения `TELEGRAM_TOKEN`

### Docker

```bash
# Сборка
docker build -t gurls-bot .

# Запуск
docker run gurls-bot
```

## Архитектура

- `cmd/bot/` - точка входа приложения
- `internal/bot/` - логика Telegram бота
- `internal/grpc/client/` - gRPC клиент для Backend
- `internal/config/` - конфигурация

## Зависимости

- GURLS-Backend должен быть запущен и доступен
- Токен Telegram бота

## Примеры использования

```
/shorten https://example.com
/shorten https://example.com title="Мой сайт" expires_in=24h
/shorten https://example.com alias=mysite
/stats mysite
/delete mysite
```