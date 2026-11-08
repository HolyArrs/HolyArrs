---
title: Logs and storage
description: Read and filter the HolyArrs log, share it safely, and find where HolyArrs keeps its files.
level: Setup
sidebar: { order: 4 }
---

## Read the log

1. Open **Settings > Logs**. The owner and admins see it.
2. Pick a **Level**: **Every line**, **Warnings and errors** or **Errors only**.
3. Type in **Text** to keep lines with those words. Case does not matter.
4. Set **From** and **To** to limit the days; **To** is included. Select **Apply**.
5. Page with **Newer** and **Older**, 100 lines at a time, newest first. You should see matching lines only.

## Share the log

1. On **Settings > Logs**, select **Download**. You get the newest lines, up to 2 MB, as a text file.
2. Attach it where you ask for help. See [Get support](../help/get-support.md).

Keys, tokens and passwords are already masked as `[hidden]`. Read the file once before posting anyway: names of titles, folders and your server address stay visible.

## Where files are kept

**Settings > This app > Storage** lists each path as HolyArrs sees it, with free space on its disk. In Docker these are paths inside the container.

| Path | Holds |
| --- | --- |
| Config folder | All HolyArrs data |
| `holyarrs.db` | Main database |
| `holyarrs-events.db` | History database |
| Backups, artwork, log folders | Inside the config folder |
| Library folders, downloader save folder | Your media and downloads |

"free space unknown" means the system did not say.

The log file is `logs/holyarrs.log` in the config folder. At 5 MB it moves to `logs/holyarrs.log.1`, so logs stay near 10 MB. `docker logs holyarrs` shows the same output.

## If it does not work

**Symptom:** the Logs page is missing.
**Cause:** only the owner and admins see it.
**Fix:** ask one of them.

**Symptom:** a path looks wrong.
**Cause:** in Docker, Storage shows container paths.
**Fix:** compare with your volume mappings. See [Folders](../start/folders.md).

## For experts

<details class="ha-more"><summary>How levels are guessed</summary>

The level comes from words in the line. "failed", "crashed" and "error" mark errors. "could not", "refused" and "timed out" mark warnings.

</details>

<details class="ha-more"><summary>What is masked</summary>

Every line is masked when written, and again when read or downloaded:

- Every saved key, token, password and webhook link, also link-escaped and base64 forms.
- Old values. When a secret changes or is removed, both log files are rewritten with it masked before the old value is dropped.
- Names and passwords in links: `http://user:pass@host` becomes `http://[hidden]@host`.
- Link parameters such as `apikey=`, `token=`, `passkey=`, `password=`.
- Pairs with a secret name: API key, token, password, secret, cookie, authorization, setup code. Quoted values are masked to the closing quote.
- Bearer and basic tokens, HolyArrs API keys (`hak_...`), the first-run setup code and long token-like strings.

Line breaks and terminal colour codes inside values are shown escaped, so a value cannot fake a new line. Saved values of 4 or 5 characters are masked only as a whole word, and never when they are a common word like `admin` or `test`.

</details>

<details class="ha-more"><summary>API</summary>

| Route | Who | What |
|---|---|---|
| `GET /api/v1/app/storage` | **Change settings** | Paths with free space |
| `GET /api/v1/logs?level=&q=&from=&to=&offset=&limit=` | Owner, admins | Lines and a total |
| `GET /api/v1/logs/download` | Owner, admins | Newest 2 MB as plain text |

API keys need the admin scope for all three.

</details>

## Related

- [Get support](../help/get-support.md)
- [Health checks](health.md)
- [Backups](backups.md)
