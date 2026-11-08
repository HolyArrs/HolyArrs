# Watch health from another container

You want a small outside tool to check whether HolyArrs is healthy, without giving it access to anything else. The health companion does that. It reads the health states once and prints how many parts are off, starting, running, retrying or failed.

## Steps

1. Build the companion from `app/backend/cmd/holyarrs-companion` in the source.
2. As the owner, create an API key with **health_read** as its only scope.
3. Save the key, which is shown once, in a private file outside the source folder.
4. Mount that file read-only at `/run/secrets/holyarrs-key` in the companion's container.
5. Set `HOLYARRS_URL` to your HolyArrs address, such as `http://your-server:6262`. Add your reverse proxy's path if you use one.
6. Run the companion. It prints the counts and exits.

## Settings explained

**The health_read scope.** A key with only this scope reads the health states and nothing else. It cannot see titles, settings or other keys, cannot change anything and cannot widen its own scope. Do not give the companion a broader key.

**Keep the key out of sight.** Never pass it as an argument or put it in an image, an address, a log, a compose file or source control.

**Limits.** The companion follows no redirects, waits ten seconds at most and reads at most 1 MiB. It prints counts only. Labels and raw error text stay out. Use HTTPS on any network you do not trust.

## Troubleshooting

The companion stops with a short fixed message when something is wrong:

- **"mount the companion key at /run/secrets/holyarrs-key"**: the key file is missing.
- **"set HOLYARRS_URL to the app address, without credentials or a query"**: the address is empty or wrong.
- **"the app refused the companion health read"**: the key is wrong, revoked or lacks the scope.
- **"the app did not answer within the deadline"**: HolyArrs is unreachable.

When you are done, revoke the key and delete its file. For ongoing use, give the companion its own key and swap it through the same mount.
