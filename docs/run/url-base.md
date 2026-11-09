---
title: Reverse proxy and URL base
description: Serve HolyArrs under a path such as /holyarrs behind nginx-proxy-manager, Traefik or Caddy.
level: Setup
sidebar: { order: 6 }
---

Put HolyArrs at an address like `https://home.example/holyarrs` behind your reverse proxy.

A **reverse proxy** is the web server in front of your apps that sends each address to the right container. The **URL base** is the path HolyArrs lives under.

## Set the path

1. Add the path to your Compose file:

   ```yaml
   services:
     holyarrs:
       environment:
         - HOLYARRS_URL_BASE=/holyarrs
   ```

2. Restart the container.
3. Add a proxy rule from [Proxy examples](#proxy-examples).
4. Open `https://home.example/holyarrs/`. You should see the sign-in page. **Settings > This app** shows the path in use.

:::caution
Your proxy must pass the path on unchanged. If it strips `/holyarrs`, every page says not found.
:::

## Choices

### Where to set the path

| Where | Takes effect |
|---|---|
| `HOLYARRS_URL_BASE` | On start. Wins over the others. |
| `-url-base /holyarrs` flag | On start, when the variable is not set. |
| **Settings > This app** | On the next start, only when the variable and flag are both unset. |

### Proxy examples

**nginx-proxy-manager.** Make a proxy host for your domain, then add a custom location:

- Location: `/holyarrs`
- Forward hostname `holyarrs`, port `6262`. No slash at the end, or the path is stripped.
- In that location's gear box:

```nginx
proxy_set_header Host $host;
proxy_set_header X-Real-IP $remote_addr;
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
proxy_buffering off;
proxy_read_timeout 1h;
```

**Traefik.** Do not add a StripPrefix middleware.

```yaml
labels:
  - traefik.enable=true
  - traefik.http.routers.holyarrs.rule=Host(`home.example`) && PathPrefix(`/holyarrs`)
  - traefik.http.services.holyarrs.loadbalancer.server.port=6262
```

**Caddy.** Use `handle`. `handle_path` strips the path.

```caddy
home.example {
	handle /holyarrs* {
		reverse_proxy holyarrs:6262 {
			flush_interval -1
		}
	}
}
```

### Host names and proxy addresses

- If people reach HolyArrs by a host name, add it to `HOLYARRS_ALLOWED_HOSTS`.
- If the proxy adds `X-Forwarded-For`, add the proxy's address to `HOLYARRS_TRUSTED_PROXIES`. Without it, sign-in cannot tell who is on your home network.

## If it does not work

| Symptom | Cause | Fix |
|---|---|---|
| Every page says not found | The proxy strips the path | Remove the strip setting or the trailing slash. |
| Live updates never arrive | The proxy buffers responses | Turn buffering off (`proxy_buffering off`, `flush_interval -1`). |
| A saved path locked you out | The saved path is wrong | Start with `HOLYARRS_URL_BASE=/` to ignore it. |
| Sign-in treats you as outside your network | The proxy is not trusted | Send `X-Real-IP` and `X-Forwarded-For`, and set `HOLYARRS_TRUSTED_PROXIES`. |

More fixes: [Troubleshooting](../../help/troubleshooting/).

## For experts

<details class="ha-more"><summary>Path rules</summary>

- Allowed characters: letters, digits, `-`, `_`, `.` and `/`. No `..` or `//`.
- `holyarrs`, `/holyarrs` and `/holyarrs/` all mean `/holyarrs`.
- A saved path that is not valid is ignored.
- Everything moves under the path: pages, the API, live updates, sign-in, and links HolyArrs builds (invite links, the calendar feed).
- `/healthz` and `/readyz` also answer at the root, so the image's own health check keeps working.
- Any other address outside the path answers not found.

</details>

<details class="ha-more"><summary>What the proxy must do</summary>

| Need | Why |
|---|---|
| Pass the path unchanged | HolyArrs routes by the full path. |
| Keep the `Host` header | Host checks and built links use it. |
| No response buffering | Live updates stream over one open response. |
| Long read timeout (1 h) | The live update stream stays open. |

</details>

## Related

- [Install](../../start/install/)
- [Sign-in](../../users/sign-in/)
- [Environment variables](../../reference/env-vars/)
