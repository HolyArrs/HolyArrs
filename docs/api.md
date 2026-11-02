# The API for other apps

Settings > API for other apps shows everything a script or another app needs, inside HolyArrs (no outside site is loaded):

1. **Make a key** in Settings > Sign-in > API keys, with only the scopes it needs. The key is shown once.
2. **Send it** in the `X-Api-Key` header with every call. Never put it in the address.
3. **The base address** is shown with a copy button, for example `http://10.10.25.25:18730/api/v1` (with a URL base: `http://host/holyarrs/api/v1`).

The page also lists the scopes (read, write, admin, control, all) in plain words, example calls ready to copy, and every route from the built-in OpenAPI file with its summary, scope and the permission it needs, with a search box. **The OpenAPI file** button opens `/api/v1/openapi.json` (signed in), for tools that read OpenAPI.

## Example

```
export HOLYARRS_KEY=...your key...
curl -H "X-Api-Key: $HOLYARRS_KEY" "http://10.10.25.25:18730/api/v1/health"
curl -X POST -H "X-Api-Key: $HOLYARRS_KEY" "http://10.10.25.25:18730/api/v1/schedules/feed_check/run"
```

Answers are JSON; errors are `{"error": "plain words"}` with an HTTP status. Secrets are never answered; they come back masked.
