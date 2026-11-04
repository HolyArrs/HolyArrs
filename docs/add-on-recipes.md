# A health companion on the API

The example at `examples/health-companion` builds a separate container that reads `GET /api/v1/health/states` once and prints counts by state. It receives no database, media, app configuration or Docker-socket mount. It runs as user 1000 with a read-only filesystem, dropped capabilities and no restart policy.

As the owner, create a task-only API key with **health_read** as its only scope. Store the once-shown value in a private file outside this checkout. The key can read health states; it cannot read titles, settings or other keys, change anything or expand its own scope. Existing broader read keys remain compatible but are unnecessary for this recipe.

Set `HOLYARRS_URL` to the app address, such as `http://10.10.25.25:6262` (include a reverse proxy's path prefix when used). Set `HOLYARRS_KEY_FILE` to that private file. Run `docker compose -f examples/health-companion/compose.yaml run --rm health-companion`. The key is mounted read-only at `/run/secrets/holyarrs-key`; never pass it as an argument or put it in the image, an address, logs, compose file or source control.

Use HTTPS across an untrusted network. The companion refuses redirects, has a ten-second request deadline and caps the answer at 1 MiB. Its output contains aggregate states; labels and raw service errors stay out. A missing key, denied response, invalid answer or unreachable app fails with a fixed message.

Revoke the task's key when finished and remove only the private file created for it. For ongoing use, create a dedicated key and rotate it through the same secret mount. A tool that changes data needs its own separately designed permissions; do not give this companion a broader key.

The local acceptance harness proves both container/API access and token denials against disposable synthetic state. It does not require an existing account, real media or an outside provider.
