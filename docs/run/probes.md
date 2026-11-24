---
title: Health probes
description: Point Docker, Kubernetes and uptime monitors at the HolyArrs addresses that need no sign-in, also under a URL base and behind a sign-in proxy.
level: Setup
sidebar: { order: 3 }
---

Three addresses tell a probe whether HolyArrs is up. They need no sign-in and no API key, so they work with sign-in on.

## Which address to use

| Address | Sign-in | Answers 200 when | Use it for |
|---|---|---|---|
| `/ping` | none | the program answers, even while starting or after a failed start | liveness, uptime monitors |
| `/healthz` | none | the database answers within 2 seconds, or while starting and updating its data | the Docker health check, a liveness probe that also checks the database |
| `/readyz` | none | HolyArrs has fully started and serves pages | readiness, load balancers |
| `/api/v1/health` | sign-in with the manage-library permission | it lists the health warnings | the Status page |
| `/health` | sign-in | it is a page in the web app | people, not probes |

`GET` and `HEAD` both work on the three open addresses.

> Probing `/api/v1/health` without signing in answers 401. Use `/healthz`, `/readyz` or `/ping` instead. To read the warning states from a script, make an API key with only the `health_read` scope and call `/api/v1/health/states` with it in the `X-Api-Key` header.

### What each one answers

| State | `/ping` | `/healthz` | `/readyz` |
|---|---|---|---|
| Running | 200 `{"status":"OK"}` | 200 `ok` | 200 `ready` |
| Database busy or locked for more than 2 s | 200 | 503 `database not answering` | 200 |
| Starting | 200 | 200 `{"state":"starting"}` | 503, `Retry-After: 10` |
| Updating its data after an update | 200 | 200 `{"state":"updating"}` | 503, `Retry-After: 10` |
| Did not start (failed update, master key problem, full disk) | 200 | 503 `{"state":"failed"}` | 503 |

A data update can take minutes on a big library. `/healthz` and `/ping` stay 200 during it, so nothing restarts HolyArrs halfway. When the start fails, the page at any address and the log say why. Restarting does not fix those, so keep liveness on `/ping` and let readiness keep the pod out of service until you fix the cause.

## URL base

With a URL base such as `/holyarrs`, the three addresses answer in both places:

- at the root: `/ping`, `/healthz`, `/readyz`
- under the base: `/holyarrs/ping`, `/holyarrs/healthz`, `/holyarrs/readyz`

Use the root for probes inside the container or the pod. Use the base when the probe goes through your reverse proxy and the proxy only forwards the base. See [Reverse proxy and URL base](url-base.md).

## Docker

The image already has a health check. It runs `/holyarrs -healthcheck` every 30 seconds, which calls `/healthz` at the root, so it keeps working with a URL base. `docker ps` shows the result. You do not need to add anything.

The image has no shell or `curl`, so a health check of your own must use the same command. To change the timing in Compose:

```yaml
services:
  holyarrs:
    image: holyarrs/holyarrs:latest
    healthcheck:
      test: ["CMD", "/holyarrs", "-healthcheck"]
      interval: 30s
      timeout: 10s
      start_period: 30s
      retries: 3
```

`HOLYARRS_HEALTH_URL` sets the base address the command calls (it adds `/healthz`), for example `http://127.0.0.1:7000` when HolyArrs listens on another port inside the container.

## Kubernetes

Point the probes straight at the pod. They do not pass through your ingress or a sign-in proxy.

```yaml
containers:
  - name: holyarrs
    image: holyarrs/holyarrs:latest
    ports:
      - name: http
        containerPort: 6262
    livenessProbe:
      httpGet:
        path: /ping
        port: http
      periodSeconds: 30
      timeoutSeconds: 5
      failureThreshold: 3
    readinessProbe:
      httpGet:
        path: /readyz
        port: http
      periodSeconds: 10
      timeoutSeconds: 5
      failureThreshold: 3
```

- **Liveness on `/ping`** restarts the pod only when the program stops answering. Use `/healthz` instead if you also want a restart when the database stops answering; keep `failureThreshold` at 3 or more so a short lock does not restart it.
- **Readiness on `/readyz`** keeps traffic away while HolyArrs starts, updates its data, or after a failed start.
- **A startup probe** is not needed, because `/ping` and `/healthz` answer 200 while starting. If your cluster rules require one, use `/ping`:

```yaml
    startupProbe:
      httpGet:
        path: /ping
        port: http
      periodSeconds: 5
      failureThreshold: 60
```

### Helm values

Charts built on the common app-template take the same probe blocks under the container's `probes` key:

```yaml
probes:
  liveness:
    enabled: true
    custom: true
    spec:
      httpGet:
        path: /ping
        port: 6262
      periodSeconds: 30
      failureThreshold: 3
  readiness:
    enabled: true
    custom: true
    spec:
      httpGet:
        path: /readyz
        port: 6262
      periodSeconds: 10
  startup:
    enabled: false
```

## Behind a sign-in proxy

Authelia, Authentik, oauth2-proxy and other forward-auth proxies sign people in before the request reaches HolyArrs. Probes that go through the proxy, such as an uptime monitor on your public address, then get the proxy's sign-in page instead of HolyArrs. Let the three addresses through without sign-in. They show nothing private.

Kubernetes probes and the Docker health check call HolyArrs directly, so they need none of this.

**Authelia**, in `access_control`, above your other rules for the HolyArrs domain:

```yaml
access_control:
  rules:
    - domain: holyarrs.example.com
      resources:
        - '^/(holyarrs/)?(ping|healthz|readyz)$'
      policy: bypass
```

**Authentik**, on the proxy provider for HolyArrs, under **Unauthenticated Paths**:

```
^/(holyarrs/)?(ping|healthz|readyz)$
```

**oauth2-proxy**:

```
--skip-auth-route=GET=^/(holyarrs/)?(ping|healthz|readyz)$
```

**nginx with `auth_request`**, one block per address:

```nginx
location = /healthz {
    auth_request off;
    proxy_pass http://holyarrs:6262;
}
```

**Caddy with `forward_auth`**:

```caddy
holyarrs.example.com {
    @probes path /ping /healthz /readyz
    handle @probes {
        reverse_proxy holyarrs:6262
    }
    handle {
        forward_auth authelia:9091 {
            uri /api/authz/forward-auth
            copy_headers Remote-User Remote-Groups Remote-Email Remote-Name
        }
        reverse_proxy holyarrs:6262
    }
}
```

Drop `(holyarrs/)?` from the patterns when you use no URL base, or change it to your own base.

## Coming from another app

| You probed | In HolyArrs |
|---|---|
| Sonarr, Radarr, Lidarr or Prowlarr `/ping` | `/ping`, same answer |
| Seerr or Overseerr `/api/v1/status` | `/readyz` |
| Jellyfin `/health` | `/healthz` |
| a plain TCP check on the port | still works; `/ping` also tells you the program answers |
