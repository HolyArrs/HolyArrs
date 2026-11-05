# Running under a path (URL base) behind a reverse proxy

HolyArrs can live at a path such as `http://home.example/holyarrs` instead of its own host or port.

## Turn it on

Set the base when HolyArrs starts, either way:

- environment: `HOLYARRS_URL_BASE=/holyarrs`
- flag: `-url-base /holyarrs`

The base is a plain path (letters, digits, `-`, `_`, `.` and `/`; no `..` or `//`). `holyarrs`, `/holyarrs` and `/holyarrs/` all mean `/holyarrs`. Settings > This app shows the base in use.

You can also save the base from the app (`PUT /api/v1/settings/url-base` with `{"url_base": "/holyarrs"}`; the Settings field uses it). A saved base takes effect on the next start, and only when the environment and the flag are not set. To ignore a saved base, start with `HOLYARRS_URL_BASE=/`. A saved base that is not a plain path is ignored, so a bad save never locks you out.

With a base set, everything is under it: the pages, scripts and styles, the API (`/holyarrs/api/v1/...`), the live stream (`/holyarrs/live`), the health check (`/holyarrs/healthz`; the root `/healthz` and `/readyz` also answer, and the image's own health check calls the root), downloads, the sign-in cookie (its path is the base) and sign-in redirects. `/holyarrs` alone goes to `/holyarrs/`. Any other address outside the base answers 404. Redirects only ever point inside the app, never to another site. The OpenAPI file names the base as its server, and links the app builds (invite links, the calendar feed, the API page's base address) carry it.

The proxy must pass the path on unchanged (do not strip `/holyarrs`), keep the `Host` header, and not buffer the live stream. If the proxy adds `X-Forwarded-For`, add its address to `HOLYARRS_TRUSTED_PROXIES`; if people reach HolyArrs by a host name, add it to `HOLYARRS_ALLOWED_HOSTS`.

## nginx-proxy-manager

Proxy host for your domain, then Custom locations > Add location:

- Location: `/holyarrs`
- Forward hostname / IP: `holyarrs` (the container), port `6262`
- In the gear (advanced) box of that location:

```
proxy_set_header Host $host;
proxy_set_header X-Real-IP $remote_addr;
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
proxy_buffering off;
proxy_read_timeout 1h;
```

Do not add a trailing slash to the forward address (that would strip the path).

Keep both address lines. With only `X-Real-IP`, nginx passes on whatever `X-Forwarded-For` the visitor sent; HolyArrs then sees two different addresses, cannot tell which one is real, and treats the visitor as outside your network.

## Traefik (Docker labels)

```
labels:
  - traefik.enable=true
  - traefik.http.routers.holyarrs.rule=Host(`home.example`) && PathPrefix(`/holyarrs`)
  - traefik.http.services.holyarrs.loadbalancer.server.port=6262
```

No StripPrefix middleware: HolyArrs expects the base in the path. Traefik streams by default.

## Caddy

```
home.example {
	handle /holyarrs* {
		reverse_proxy holyarrs:6262 {
			flush_interval -1
		}
	}
}
```

Use `handle`, not `handle_path` (which strips the path).

## Docker compose

```
services:
  holyarrs:
    image: holyarrs
    environment:
      - HOLYARRS_URL_BASE=/holyarrs
```
