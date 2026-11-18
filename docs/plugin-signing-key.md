# Plugin signing key

A checklist for the maintainer. Do every step yourself, on your own PC.

## What the key is

HolyArrs only trusts the plugin list and the plugin blocklist when they are
signed with this key. The app holds the public half. The private half signs
new lists. Whoever has the private file and its passphrase can tell every
HolyArrs install which plugins are safe, so it never leaves your hands.

There are two keys: a main key, used for signing, and a backup key, kept for
the day the main key is lost or stolen. The app trusts both.

## 1. Make the main key (offline)

1. Open PowerShell in the repo folder.
2. Build the tool while you are online, so the run itself needs no network:

   ```powershell
   go build -o $env:USERPROFILE\pluginkey.exe ./app/backend/tools/pluginkey
   ```

3. Turn off Wi-Fi and unplug the network cable.
4. Make the key. Pick a long passphrase (at least 12 characters) and type it
   twice when asked:

   ```powershell
   & $env:USERPROFILE\pluginkey.exe keygen -out $env:USERPROFILE\plugin-key-main
   ```

   (`go run ./app/backend/tools/pluginkey keygen -out ...` works the same if you
   already have the Go modules downloaded.)

5. Write down the fingerprint it prints.

## 2. Make the backup key the same way

Use a different passphrase:

```powershell
& $env:USERPROFILE\pluginkey.exe keygen -out $env:USERPROFILE\plugin-key-backup
```

The tool never overwrites a key, so each run needs its own folder.

## 3. Store them safely

- Put both `plugin-signing.key` files and both passphrases in **two separate
  safe places**, for example a password manager and an offline USB stick.
- Keep each passphrase apart from its key file where you can.
- Never put them in the repo, a cloud chat, a shared drive or anywhere an
  agent can read.
- After copying, delete the folders from your PC and turn the network back on.

## 4. Send the public keys

Send only the contents of the two `plugin-signing.pub` files (one line each)
to the session, saying which is main and which is backup. They get pinned in
the app. They are safe to share.

## Signing a list later

```powershell
& $env:USERPROFILE\pluginkey.exe sign -key E:\plugin-key-main\plugin-signing.key -kind blocklist -in payload.json -out blocklist.json -min-counter 4
& $env:USERPROFILE\pluginkey.exe verify -pub E:\plugin-key-main\plugin-signing.pub -in blocklist.json
```

`-min-counter` is the counter of the last list you published; the tool refuses
a payload whose counter is not above it.

## If a key is lost or stolen

- **Lost (no one else has it):** sign with the backup key from now on. Make a
  new backup key and ship its public half in the next release.
- **Stolen, or you think someone saw it:** stop using it at once. Sign a new
  blocklist with the backup key, then ship a release that drops the stolen
  public key and adds a new backup key. Tell users to update.
- **Both gone:** a release has to pin two new public keys. Lists signed with
  the old keys stop working once users update.
