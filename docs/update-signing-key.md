# Update signing key

A note for the maintainer on the key that signs in-app updates (#146).

## What it signs

In-app updates read a signed list at
`https://updates.holyarrs.com/v1/manifest.json` (plus `.minisig`). Each
program the list names (`holyarrs_<version>_linux_<arch>` on the GitHub
release) carries its own `.minisig`. The app trusts only the public keys in
`PinnedKeys` (`app/backend/internal/updates/minisign.go`). While that list
is empty the app reads GitHub releases instead: the update notice still
shows, but nothing can be installed.

The notice never depends on the list: when the list cannot be read, the
GitHub release check still says a new version exists. Automatic updating is
off by default; Ask me and Automatic are the owner's choice in Settings or
`HOLYARRS_UPDATES`.

## Making the key (once)

The key has no passphrase and no copy anywhere except the secret. One
command sends the private half straight into the secret and writes only the
public half to disk:

```sh
go run ./app/backend/tools/updatekey keygen -plain -pipe -pub deploy/release/updates/keys/main.pub \
  | gh secret set UPDATE_SIGNING_KEY --env update-signing -R HolyArrs/HolyArrs
```

`-pipe` refuses to run when its output is a terminal. Then put the second
line of `main.pub` into `PinnedKeys`; `TestPinnedKeyMatchesReleaseKeyFile`
fails until both match.

## Who signs

`.github/workflows/updates.yml` runs after HolyArrs - Linux downloads. It
writes the key to the runner's temp folder, signs each program and the list,
checks them against `main.pub`, shreds the key, adds the signatures to the
release and uploads the list to the R2 bucket behind updates.holyarrs.com.

A tag-started run never marks a release safe for Automatic. To mark one
safe (or one way, or a security release), start the workflow by hand with
those boxes ticked; the new list gets a higher serial and replaces the old.

## Rotating

Make a new key as above (remove `main.pub` first), pin the new public key
next to the old one, release, and drop the old key one release later.

## Lab test

`deploy/release/updates/lab_e2e.py` runs an old lab build against a local
signed list made with a throwaway test key, on a lab port, and checks Off,
Ask me, Update now, Go back, Automatic (safe and not safe) and the automatic
rollback of a release that does not start.
