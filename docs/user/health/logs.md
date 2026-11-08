# Logs and storage

You want to see what HolyArrs did, or find where it keeps its files. Both are in Settings.

## Read the log

1. Open **Settings > Logs**. The owner and admins can see it.
2. Pick a **Level**: every line, warnings and errors, or errors only.
3. Type in **Text** to show only lines with those words. Case does not matter.
4. Set **From** and **To** to limit the days. To is included.
5. Use **Newer** and **Older** to page through 100 lines at a time, newest first.

To share a log, click **Download the log**. You get the newest lines, at most 2 MB, as a text file. The same button is on **Settings > This app**.

HolyArrs guesses the level from the words in a line. "failed", "crashed" and "error" mark errors. "could not", "refused" and "timed out" mark warnings.

## Where files are kept

**Settings > This app > Storage** lists the paths as HolyArrs sees them. In Docker these are paths inside the container.

- The config folder
- The main database `holyarrs.db` and the history database `holyarrs-events.db`, with their sizes
- The backups, artwork and log folders
- Every Library folder, and the Downloader save folder when one is set

Each path shows the free space on its disk. "free space unknown" means the system did not say.

The log file is `logs/holyarrs.log` in the config folder. At 5 MB it moves to `logs/holyarrs.log.1`, so logs never take more than about 10 MB. You can also read the output with `docker logs holyarrs`.

## Secrets stay hidden

Every line is masked when it is written and again when you read or download it. Masked values show as `[hidden]`. This covers:

- Every saved key, token, password and webhook link, also in link-escaped and base64 forms.
- Old values. When you change or remove a secret, HolyArrs rewrites both log files with it masked before it forgets the old value.
- Names and passwords in links. `http://user:pass@host` becomes `http://[hidden]@host`.
- Secret link parameters such as `apikey=`, `token=`, `passkey=` and `password=`.
- Pairs with a secret name, such as API key, token, password, secret, cookie, authorization and setup code. Quoted values are masked up to the closing quote.
- Bearer and basic tokens, HolyArrs API keys (`hak_...`), the first-run setup code and long token-like strings.

Line breaks and terminal color codes inside logged values are shown escaped, so a value cannot fake a new line.

Very short saved values (four or five characters) are masked only where they stand alone as a word, and never when they are a common word like `admin` or `test`.

## API

| Route | Who | What |
|---|---|---|
| `GET /api/v1/app/storage` | People who may change settings | The paths above with free space |
| `GET /api/v1/logs?level=&q=&from=&to=&offset=&limit=` | Owner, admins | Lines and a total |
| `GET /api/v1/logs/download` | Owner, admins | The newest 2 MB as plain text |

API keys need the admin scope for all three.

## Troubleshooting

**The Logs page is missing.** Only the owner and admins see it. Ask one of them.

**A path looks wrong.** In Docker, Storage shows paths inside the container. Check your volume mappings.
