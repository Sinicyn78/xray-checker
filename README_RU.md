# Xray Checker (Fork)

Форк проекта [kutovoys/xray-checker](https://github.com/kutovoys/xray-checker) с дополнительными функциями и исправлениями для продакшн-использования.

## О проекте

`xray-checker` проверяет доступность прокси-конфигураций (VLESS, VMess, Trojan, Shadowsocks) через Xray Core, публикует метрики Prometheus и предоставляет Web UI/API для мониторинга.

Основные сценарии:

- мониторинг подписок VPN/Proxy в реальном времени;
- публичная страница статуса для клиентов;
- экспорт метрик в Prometheus + опциональный push в Pushgateway;
- интеграция с Uptime Kuma и другими системами.

## Это форк: что важно знать

- Upstream: `https://github.com/kutovoys/xray-checker`
- Этот репозиторий: `https://github.com/Sinicyn78/xray-checker`
- В форке сохранена совместимость по базовой конфигурации, но добавлены форк-специфичные улучшения (см. ниже).

### Изменения форка относительно upstream

- добавлен лимит параллельных проверок: `PROXY_CHECK_CONCURRENCY`;
- добавлено логирование в файл: `LOG_FILE`;
- добавлены удалённые источники подписок через API (добавление/удаление/обновление URL без ручного редактирования файла);
- улучшена работа с `file://` директориями подписок и хранением состояния remote-источников;
- исправлена логика привязки статусов к прокси по `StableID`;
- улучшена обработка пустых/недоступных подписок;
- улучшен fallback-рендеринг вкладки серверов в Web UI;
- усилена валидация URL для remote-источников;
- нормализованы некорректные значения stream security.

## Возможности

- поддержка протоколов: `vless`, `vmess`, `trojan`, `shadowsocks`;
- загрузка конфигураций из нескольких источников одновременно;
- форматы источников:
  - URL подписки;
  - base64 строка;
  - `file://` JSON-файл;
  - `folder://` папка с JSON-файлами;
- методы проверки:
  - `ip` (сравнение внешнего IP);
  - `status` (проверка HTTP-ответа);
  - `download` (проверка скачиванием файла);
- метрики Prometheus:
  - `xray_proxy_status`;
  - `xray_proxy_latency_ms`;
- Web UI + REST API + Swagger (`/api/v1/docs`);
- публичный режим дашборда (`WEB_PUBLIC=true`);
- Basic Auth для API/metrics;
- кастомизация интерфейса через `WEB_CUSTOM_ASSETS_PATH`;
- запуск в Docker и как обычный бинарник.

## Архитектура (кратко)

1. Источники подписок парсятся в набор proxy-конфигов.
2. Генерируется `xray_config.json` и запускается Xray Core.
3. Checker проверяет каждый прокси (параллельно с лимитом).
4. Статусы и задержки публикуются в метрики и API.
5. При обновлениях подписок конфигурация пересобирается автоматически.

## Быстрый старт

### Docker (минимум)

```bash
docker run -d \
  --name xray-checker \
  -e SUBSCRIPTION_URL="https://example.com/subscription" \
  -p 2112:2112 \
  sinicyn/xray-checker:latest
```

Если вы ещё не публиковали образ форка:

```bash
docker build -t xray-checker:local .
docker run -d \
  --name xray-checker \
  -e SUBSCRIPTION_URL="https://example.com/subscription" \
  -p 2112:2112 \
  xray-checker:local
```

### Docker Compose

```yaml
services:
  xray-checker:
    image: sinicyn/xray-checker:latest
    container_name: xray-checker
    restart: unless-stopped
    environment:
      SUBSCRIPTION_URL: "https://example.com/subscription"
      PROXY_CHECK_METHOD: "ip"
      PROXY_CHECK_INTERVAL: "300"
      PROXY_CHECK_CONCURRENCY: "32"
      METRICS_PROTECTED: "true"
      METRICS_USERNAME: "admin"
      METRICS_PASSWORD: "change-me"
    ports:
      - "2112:2112"
```

### Запуск бинарника

```bash
go build -o xray-checker .
./xray-checker --subscription-url="https://example.com/subscription"
```

## Конфигурация

Приложение поддерживает CLI-флаги и переменные окружения. Обязательный параметр только один: источник подписки.

### Обязательные

- `SUBSCRIPTION_URL` / `--subscription-url`

Можно указать несколько источников:

- повторить `--subscription-url` несколько раз;
- или передать значения через запятую в `SUBSCRIPTION_URL`.

### Основные параметры

#### Subscription

- `SUBSCRIPTION_URL` (`--subscription-url`) - источник(и) конфигов
- `SUBSCRIPTION_UPDATE` (`--subscription-update`, default `true`)
- `SUBSCRIPTION_UPDATE_INTERVAL` (`--subscription-update-interval`, default `300`)

#### Proxy

- `PROXY_CHECK_INTERVAL` (`--proxy-check-interval`, default `300`)
- `PROXY_CHECK_CONCURRENCY` (`--proxy-check-concurrency`, default `32`) - **фича форка**
- `PROXY_CHECK_METHOD` (`--proxy-check-method`, `ip|status|download`, default `ip`)
- `PROXY_IP_CHECK_URL` (`--proxy-ip-check-url`)
- `PROXY_STATUS_CHECK_URL` (`--proxy-status-check-url`)
- `PROXY_DOWNLOAD_URL` (`--proxy-download-url`)
- `PROXY_DOWNLOAD_TIMEOUT` (`--proxy-download-timeout`, default `60`)
- `PROXY_DOWNLOAD_MIN_SIZE` (`--proxy-download-min-size`, default `51200`)
- `PROXY_TIMEOUT` (`--proxy-timeout`, default `30`)
- `PROXY_RESOLVE_DOMAINS` (`--proxy-resolve-domains`, default `false`)
- `SIMULATE_LATENCY` (`--simulate-latency`, default `true`)

#### Xray

- `XRAY_START_PORT` (`--xray-start-port`, default `10000`)
- `XRAY_LOG_LEVEL` (`--xray-log-level`, `debug|info|warning|error|none`, default `none`)

#### Metrics / API

- `METRICS_HOST` (`--metrics-host`, default `0.0.0.0`)
- `METRICS_PORT` (`--metrics-port`, default `2112`)
- `METRICS_BASE_PATH` (`--metrics-base-path`, default `""`)
- `METRICS_PROTECTED` (`--metrics-protected`, default `false`)
- `METRICS_USERNAME` (`--metrics-username`)
- `METRICS_PASSWORD` (`--metrics-password`)
- `METRICS_INSTANCE` (`--metrics-instance`)
- `METRICS_PUSH_URL` (`--metrics-push-url`, формат: `https://user:pass@host:port`)

#### Web

- `WEB_SHOW_DETAILS` (`--web-show-details`, default `false`)
- `WEB_PUBLIC` (`--web-public`, default `false`)
- `WEB_CUSTOM_ASSETS_PATH` (`--web-custom-assets-path`)

Ограничение: `WEB_PUBLIC=true` требует `METRICS_PROTECTED=true`.

#### Логи / режимы

- `LOG_LEVEL` (`--log-level`, `debug|info|warn|error|none`, default `info`)
- `LOG_FILE` (`--log-file`) - **фича форка**
- `RUN_ONCE` (`--run-once`, default `false`)

## Эндпоинты

Базовый адрес: `http://localhost:2112`.

- `GET /health` - healthcheck
- `GET /metrics` - метрики Prometheus
- `GET /api/v1/status` - агрегированный статус
- `GET /api/v1/proxies` - список прокси
- `GET /api/v1/proxies/{stableID}` - прокси по ID
- `GET /api/v1/public/proxies` - публичный безопасный список
- `GET /api/v1/config` - активная конфигурация
- `GET /api/v1/system/info` - версия/uptime
- `GET /api/v1/system/ip` - текущий определённый IP
- `GET /api/v1/openapi.yaml` - OpenAPI спецификация
- `GET /api/v1/docs` - Swagger UI

### API удалённых подписок (фича форка)

Доступно при использовании `SUBSCRIPTION_URL` с `file://` источником (файл или директория).

- `GET /api/v1/subscriptions/remote` - состояние источников
- `POST /api/v1/subscriptions/remote` - добавить URL(ы)
- `DELETE /api/v1/subscriptions/remote?id=<id|url>` - удалить источник
- `POST /api/v1/subscriptions/remote/refresh` - форс-обновление
- `PUT /api/v1/subscriptions/remote/interval` - изменить интервал обновления

Пример добавления источников:

```bash
curl -u admin:change-me \
  -H "Content-Type: application/json" \
  -X POST \
  -d '{"urls":["https://example.com/sub1","https://example.com/sub2"]}' \
  http://localhost:2112/api/v1/subscriptions/remote
```

## Рекомендации по методам проверки

- `ip`: минимальная нагрузка, оптимально по умолчанию.
- `status`: стабильная HTTP-проверка доступности.
- `download`: проверка реального трафика и полезной нагрузки.

Для больших подписок типовой базовый профиль:

- `PROXY_CHECK_METHOD=ip`
- `PROXY_CHECK_INTERVAL=120..300`
- `PROXY_CHECK_CONCURRENCY=32..128` (подбирается по CPU/сети)

## Кастомизация Web UI

Укажите `WEB_CUSTOM_ASSETS_PATH` и положите файлы в директорию:

- `index.html` - полная замена шаблона;
- `logo.svg` - кастомный логотип;
- `favicon.ico` - кастомный фавикон;
- `custom.css` - дополнительные стили;
- любые другие файлы будут доступны по `/static/<filename>`.

## Сборка и разработка

```bash
go test ./...
go build ./...
```

Локальный запуск с debug-логом:

```bash
go run . \
  --subscription-url="https://example.com/subscription" \
  --log-level=debug
```

## Совместимость с upstream

- Базовое поведение ENV/API в основном совместимо с upstream.
- Миграция с upstream обычно не требует переписывания конфигов, если не используются fork-only фичи.
- Для remote-менеджера нужна доступная на запись директория `file://`.

## Безопасность

Минимальные рекомендации для продакшна:

- включайте `METRICS_PROTECTED=true`;
- задавайте свои `METRICS_USERNAME` и `METRICS_PASSWORD`;
- не включайте `WEB_SHOW_DETAILS` на публичных инсталляциях;
- ставьте сервис за TLS reverse-proxy (Nginx/Caddy/Traefik).

## Лицензия

Проект распространяется под лицензией [GNU GPLv3](./LICENSE).

## Благодарности

- Оригинальный проект: [kutovoys/xray-checker](https://github.com/kutovoys/xray-checker)
- В этом репозитории поддерживается форк и его дополнительные возможности.
