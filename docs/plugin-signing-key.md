# Plugin signing key

A note for the maintainer on where the plugin signing key lives and who uses it.

## What the key is

HolyArrs trusts the plugin list and the plugin blocklist only when they are
signed with a key it knows. The app pins one public key, in
`app/backend/cmd/holyarrs/plugins_wiring.go`; the same key is in
`deploy/release/plugins/keys/main.pub`, and a test checks they match.
Whoever has the private key can tell every HolyArrs install which plugins
are safe.

Fingerprint of the current key:
`SHA256:0758fcfa9ac0e5d303c9489dd7dd4f7d13e2232ba5a13534e88d6e55d969d234`

## Where the key lives

Only in the `PLUGIN_SIGNING_KEY` secret of the `plugin-signing` environment
in `HolyArrs/HolyArrs-Plugins`. The environment only runs from `main`. The
key has no passphrase and no copy exists anywhere else: no PC, no drive, no
vault. Nobody handles it.

It was made with one command that sends the private key straight into the
secret and writes only the public key to disk:

```sh
pluginkey keygen -plain -pipe -pub deploy/release/plugins/keys/main.pub \
  | gh secret set PLUGIN_SIGNING_KEY --env plugin-signing -R HolyArrs/HolyArrs-Plugins
```

`-pipe` refuses to run when its output is a terminal, so the key is never shown.

## Who signs

The release automation runs the `init.yml`, `publish.yml` and `block.yml`
workflows in `HolyArrs/HolyArrs-Plugins`. Each run writes the key to the
runner's temp folder, signs, runs `pluginkey check` on the result and
shreds the key, even when a step fails. Nobody takes part. See
[Publish plugins](dev/releasing.md#publish-plugins).

`pluginkey sign` still opens an older sealed key with `-pass-env`, but the
current key needs none.

## Rotating the key

There is no backup key. A rotation is a new key in a release:

1. Run the command above again (remove `keys/main.pub` first). It replaces
   the secret.
2. Put the new public key in `pluginKeys` in `plugins_wiring.go`.
3. Release HolyArrs, then re-sign the lists with `init.yml` or the next
   `publish.yml` run.

Installs on the older release keep trusting only the old key until they
update, so they see no new lists in between. If the key leaks, do the same
and tell users to update.
