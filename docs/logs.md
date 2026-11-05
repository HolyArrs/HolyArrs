# Logs and storage (This app)

## Where things are kept

Settings > This app > Storage lists, as HolyArrs sees the paths (inside the container in Docker):

- the config folder, the main database (`holyarrs.db`) and the history database (`holyarrs-events.db`) with their sizes,
- the backups, artwork and log folders,
- every Library folder, and the Downloader save folder when one is set,

each with the free space on its disk ("free space unknown" where the system does not say). People who may change settings see it; `GET /api/v1/app/storage` answers the same list.

## The log viewer

Settings > Logs (owner and admins) reads the app log in the browser:

- **Level**: every line, warnings and errors, or errors only. HolyArrs reads the level from the line's words ("failed", "crashed", "error" are errors; "could not", "refused", "timed out" are warnings).
- **Text**: lines that hold the words, any case.
- **From** and **To**: days (To is included).
- **Newer** and **Older** page through 100 lines at a time, newest first.
- **Download the log** (also on This app) saves the newest lines, at most 2 MB, as a text file.

The log lives in the config folder: `logs/holyarrs.log`, moved to `logs/holyarrs.log.1` at 5 MB, so it never takes more than about 10 MB. The program's own output (`docker logs holyarrs`) is masked the same way. Control characters in logged values (line breaks, carriage returns, terminal colour codes) are shown escaped, as `
`, `` or ``, in the file and in the output, so a value cannot make a fake line.

## Secrets are never shown

Every line is masked twice: when it is written to the log file and again whenever the viewer or the download reads it. Masked, as `[hidden]`:

- every value in the encrypted vault (keys, tokens, passwords of linked apps, webhook links), wherever it appears, also in its link-escaped and base64 forms. The vault hands a value to the masker as it is saved, before the save finishes, so a new secret never reaches the file; the list is also read at start, every minute and on every read of the log,
- old values too: the list of masked values only grows while HolyArrs runs. When a secret is changed or removed, the vault keeps its old value sealed (encrypted, never in the clear) until the masker has rewritten both log files with it masked; only then is it forgotten. So a rotated key never shows again, not after a restart either,
- user name and password in links (`http://user:pass@host` becomes `http://[hidden]@host`),
- secret query parameters (`apikey=`, `token=`, `passkey=`, `password=` and the like),
- `key: value` and `key=value` pairs with a secret name (API key, token, password, secret, cookie, authorization, setup code),
- bearer and basic tokens, HolyArrs API keys (`hak_...`), the first-run setup code and long token-like runs of letters and digits.

- quoted values after a secret name are masked up to the closing quote (`password="two words"`).

Vault values of four or five characters are masked only where they stand alone as a word, and never when they are a common word (such as `admin`, `true` or `test`); shorter values are left to the patterns above.

## API

| Route | Who | What |
|---|---|---|
| `GET /api/v1/app/storage` | settings permission | the paths above with free space |
| `GET /api/v1/logs?level=&q=&from=&to=&offset=&limit=` | owner, admin | lines `{at, level, text}` and `total` |
| `GET /api/v1/logs/download` | owner, admin | the newest 2 MB as `text/plain` |

API keys need the admin scope for all three.
