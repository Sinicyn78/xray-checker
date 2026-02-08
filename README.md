# Xray Checker (Fork)

Fork of [kutovoys/xray-checker](https://github.com/kutovoys/xray-checker) with additional production-focused fixes and features.

## About

`xray-checker` validates proxy availability (VLESS, VMess, Trojan, Shadowsocks) through Xray Core, exports Prometheus metrics, and provides a Web UI/API for monitoring.

Typical use cases:

- real-time VPN/proxy subscription monitoring;
- public status page for users;
- Prometheus scraping with optional Pushgateway push;
- integrations with Uptime Kuma and similar systems.

## Fork Notes

- Upstream: `https://github.com/kutovoys/xray-checker`
- This repository: `https://github.com/Sinicyn78/xray-checker`
- Core upstream behavior is preserved, with fork-specific improvements listed below.

### Fork changes vs upstream

- added concurrent check limiter: `PROXY_CHECK_CONCURRENCY`;
- added file logging: `LOG_FILE`;
- added remote subscription sources via API (add/remove/refresh URLs without manual file edits);
- improved `file://` subscription directory handling and remote state location;
- fixed proxy status mapping by `StableID` to prevent mismatch;
- improved empty/unavailable subscription handling;
- improved fallback rendering in Web UI server tab;
- improved remote URL validation;
- normalized invalid stream security values.

## Features

- protocols: `vless`, `vmess`, `trojan`, `shadowsocks`;
- multiple subscription sources merged into one runtime set;
- supported sources:
  - subscription URL;
  - base64 string;
  - `file://` JSON file;
  - `folder://` directory with JSON files;
- check methods:
  - `ip` (egress IP comparison);
  - `status` (HTTP status endpoint);
  - `download` (minimum download size verification);
- Prometheus metrics:
  - `xray_proxy_status`;
  - `xray_proxy_latency_ms`;
- Web UI + REST API + Swagger (`/api/v1/docs`);
- public dashboard mode (`WEB_PUBLIC=true`);
- Basic Auth for API/metrics;
- UI customization with `WEB_CUSTOM_ASSETS_PATH`;
- Docker and native binary usage.

## Architecture (short)

1. Subscription sources are parsed into proxy configs.
2. `xray_config.json` is generated and Xray Core starts.
3. Checker validates each proxy (parallelized with concurrency limit).
4. Status/latency are exposed via metrics and API.
5. On subscription updates, config is rebuilt automatically.

## Quick Start

### Docker (minimum)

```bash
docker run -d \
  --name xray-checker \
  -e SUBSCRIPTION_URL="https://example.com/subscription" \
  -p 2112:2112 \
  sinicyn/xray-checker:latest
```

If your fork image is not published yet:

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

### Binary

```bash
go build -o xray-checker .
./xray-checker --subscription-url="https://example.com/subscription"
```

## Configuration

The app supports both CLI flags and environment variables. Only one field is required: subscription source.

### Required

- `SUBSCRIPTION_URL` / `--subscription-url`

You can pass multiple sources:

- repeat `--subscription-url` multiple times;
- or use comma-separated values in `SUBSCRIPTION_URL`.

### Main options

#### Subscription

- `SUBSCRIPTION_URL` (`--subscription-url`) - config source(s)
- `SUBSCRIPTION_UPDATE` (`--subscription-update`, default `true`)
- `SUBSCRIPTION_UPDATE_INTERVAL` (`--subscription-update-interval`, default `300`)

#### Proxy

- `PROXY_CHECK_INTERVAL` (`--proxy-check-interval`, default `300`)
- `PROXY_CHECK_CONCURRENCY` (`--proxy-check-concurrency`, default `32`) - **fork feature**
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
- `METRICS_PUSH_URL` (`--metrics-push-url`, format: `https://user:pass@host:port`)

#### Web

- `WEB_SHOW_DETAILS` (`--web-show-details`, default `false`)
- `WEB_PUBLIC` (`--web-public`, default `false`)
- `WEB_CUSTOM_ASSETS_PATH` (`--web-custom-assets-path`)

Constraint: `WEB_PUBLIC=true` requires `METRICS_PROTECTED=true`.

#### Logging / run mode

- `LOG_LEVEL` (`--log-level`, `debug|info|warn|error|none`, default `info`)
- `LOG_FILE` (`--log-file`) - **fork feature**
- `RUN_ONCE` (`--run-once`, default `false`)

## Endpoints

Default bind: `http://localhost:2112`.

- `GET /health` - healthcheck
- `GET /metrics` - Prometheus metrics
- `GET /api/v1/status` - aggregated status
- `GET /api/v1/proxies` - proxy list
- `GET /api/v1/proxies/{stableID}` - proxy by ID
- `GET /api/v1/public/proxies` - public-safe proxy view
- `GET /api/v1/config` - effective runtime config
- `GET /api/v1/system/info` - version/uptime
- `GET /api/v1/system/ip` - current detected IP
- `GET /api/v1/openapi.yaml` - OpenAPI spec
- `GET /api/v1/docs` - Swagger UI

### Remote subscription API (fork feature)

Available when `SUBSCRIPTION_URL` uses a `file://` source (file or directory).

- `GET /api/v1/subscriptions/remote` - get remote source state
- `POST /api/v1/subscriptions/remote` - add URL(s)
- `DELETE /api/v1/subscriptions/remote?id=<id|url>` - remove source
- `POST /api/v1/subscriptions/remote/refresh` - force refresh
- `PUT /api/v1/subscriptions/remote/interval` - set refresh interval

Add source example:

```bash
curl -u admin:change-me \
  -H "Content-Type: application/json" \
  -X POST \
  -d '{"urls":["https://example.com/sub1","https://example.com/sub2"]}' \
  http://localhost:2112/api/v1/subscriptions/remote
```

## Check method guidance

- `ip`: lowest overhead, good default.
- `status`: stable HTTP-level availability check.
- `download`: validates real transfer path and throughput.

For large subscriptions, a typical baseline:

- `PROXY_CHECK_METHOD=ip`
- `PROXY_CHECK_INTERVAL=120..300`
- `PROXY_CHECK_CONCURRENCY=32..128` (tune to CPU/network)

## Web UI customization

Set `WEB_CUSTOM_ASSETS_PATH` and place files in that directory:

- `index.html` - full template override;
- `logo.svg` - custom logo;
- `favicon.ico` - custom favicon;
- `custom.css` - extra style overrides;
- any extra file is served at `/static/<filename>`.

## Build and development

```bash
go test ./...
go build ./...
```

Local debug run:

```bash
go run . \
  --subscription-url="https://example.com/subscription" \
  --log-level=debug
```

## Upstream compatibility

- Most base ENV/API behavior remains upstream-compatible.
- Migration from upstream usually requires no config rewrite unless using fork-only features.
- For remote manager usage, ensure writable `file://` storage path.

## Security

Recommended production baseline:

- set `METRICS_PROTECTED=true`;
- set custom `METRICS_USERNAME` and `METRICS_PASSWORD`;
- avoid `WEB_SHOW_DETAILS` on public deployments;
- place service behind TLS reverse proxy (Nginx/Caddy/Traefik).

## License

Licensed under [GNU GPLv3](./LICENSE).

## Credits

- Original project: [kutovoys/xray-checker](https://github.com/kutovoys/xray-checker)
- This repository maintains the fork and additional features.
