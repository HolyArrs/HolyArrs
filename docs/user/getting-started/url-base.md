# Run HolyArrs under a path

You want HolyArrs at an address like `https://home.example/holyarrs` behind a
reverse proxy, instead of on its own port or host name.

## Steps

1. Set the path when HolyArrs starts:

   ```yaml
   services:
     holyarrs:
       environment:
         - HOLYARRS_URL_BASE=/holyarrs
   ```

   Or start it with `-url-base /holyarrs`.
2. Point your proxy at HolyArrs. Use one of the examples below.
3. Open `https://home.example/holyarrs/`. Settings > This app shows the path
   in use.

You can also save the path in Settings > This app. A saved path takes effect
on the next start, and only when the environment variable and the flag are
not set.

## Settings explained

- The path may use letters, digits, `-`, `_`, `.` and `/`. No `..` or `//`.
- `holyarrs`, `/holyarrs` and `/holyarrs/` all mean `/holyarrs`.
- Everything moves under the path: pages, the API, the live updates, sign-in
  and links HolyArrs builds, such as invite links and the calendar feed.
- `/healthz` and `/readyz` also still answer at the root, so the image's own
  health check keeps working.
- Any other address outside the path answers "not found".

Your proxy must:

- pass the path on unchanged (do not strip `/holyarrs`),
- keep the `Host` header,
- not buffer responses, so live updates arrive.

If the proxy adds `X-Forwarded-For`, add its address to
`HOLYARRS_TRUSTED_PROXIES`. If people use a host name, add it to
`HOLYARRS_ALLOWED_HOSTS`. See [Install](install.md#host-names-and-reverse-proxies).

## Proxy examples

### nginx-proxy-manager

Make a proxy host for your domain, then add a custom location:

- Location: `/holyarrs`
- Forward hostname: `holyarrs`, port `6262`
- In the gear box of that location:

```
proxy_set_header Host $host;
proxy_set_header X-Real-IP $remote_addr;
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
proxy_buffering off;
proxy_read_timeout 1h;
```

Do not add a slash at the end of the forward address. That strips the path.

### Traefik

```
labels:
  - traefik.enable=true
  - traefik.http.routers.holyarrs.rule=Host(`home.example`) && PathPrefix(`/holyarrs`)
  - traefik.http.services.holyarrs.loadbalancer.server.port=6262
```

Do not add a StripPrefix middleware.

### Caddy

```
home.example {
	handle /holyarrs* {
		reverse_proxy holyarrs:6262 {
			flush_interval -1
		}
	}
}
```

Use `handle`. `handle_path` strips the path.

## Troubleshooting

| Problem | Do this |
| --- | --- |
| Every page says not found | The proxy strips the path. Remove the strip setting or trailing slash. |
| Live updates do not arrive | Turn off buffering in the proxy. |
| A saved path locked you out | Start with `HOLYARRS_URL_BASE=/` to ignore it. A saved path that is not valid is ignored anyway. |
| Sign-in treats you as outside your network | Send both `X-Real-IP` and `X-Forwarded-For`, and trust the proxy. |
