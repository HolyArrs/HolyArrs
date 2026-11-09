---
title: Old-app API
description: Let apps built for Sonarr and Radarr, such as Seerr, Bazarr and Prowlarr, talk to HolyArrs at /sonarr and /radarr.
level: Expert
sidebar: { order: 1 }
---

Point an app that only knows Sonarr or Radarr at HolyArrs. Give it an address ending in `/sonarr` or `/radarr` and a key, and it treats HolyArrs as that app.

This is a small API made only for those apps. For your own scripts, use the [HolyArrs API](../reference/api-routes/).

## Turn it on

It is off by default. While off, every address under `/sonarr/` and `/radarr/` answers 404.

1. Open **Settings > API**.
2. On the **Requests from Seerr and Overseerr** card, switch on **Answer Seerr and Overseerr**.
3. Click **Make key** next to **Shows**. This is the Sonarr-side key. Copy it now; it is shown once.
4. Click **Make key** next to **Movies**. This is the Radarr-side key. Copy it too.
5. Note the address for each side on the card, such as `/sonarr`. Under a URL base it shows `/holyarrs/sonarr`. Use what the card shows.

Making a key asks for your password again if you have not signed in with it recently. Then set up each app: see [Old-app API clients](../reference/old-app-api-clients/).

:::caution
Seerr stores keys in plain text in its settings file. That is why **Let these apps delete files** is off by default.
:::

## Card settings

| Setting | What it does | Default |
|---|---|---|
| **Answer Seerr and Overseerr** | Turns `/sonarr` and `/radarr` on or off together. | Off |
| **Make key** / **New key** | Makes the key for that side. A new key stops the old one at once. | |
| **Remove key** | Deletes that side's key. Apps using it are refused. | |
| **Let these apps delete files** | When off, removing a title in an app leaves the files. When on, files go to the holding folder first, like every HolyArrs delete. | Off |

For each key the card shows its last 4 characters, who made it and when it was last used.

## For experts

<details class="ha-more"><summary>Sending the key</summary>

HolyArrs reads the key from the first place it finds, in this order. A wrong key in an earlier place fails, even if a later one is right.

| Order | Place | Example |
|---|---|---|
| 1 | `apikey` query value, any case | `/sonarr/api/v3/series?apikey=KEY` |
| 2 | `X-Api-Key` header | `X-Api-Key: KEY` |
| 3 | `Authorization` header, with or without `Bearer ` | `Authorization: Bearer KEY` |

```sh
curl -H "X-Api-Key: KEY" http://your-server:6262/sonarr/api/v3/system/status
```

The query form is accepted only on these addresses, and HolyArrs hides it in its own log. A reverse proxy in front may still log the full address.

</details>

<details class="ha-more"><summary>Key rules</summary>

- Each side has its own key, made only on this card. HolyArrs API keys (starting `hak_`) don't work here, and these keys work nowhere else.
- A key acts as the person who made it, who needs permission to manage the Library. Prowlarr also needs permission to change settings.
- If that person is turned off or loses the permission, apps are refused until someone makes a new key. The card warns you.

</details>

<details class="ha-more"><summary>Manage it with the HolyArrs API</summary>

Signed in with your password. Key calls need a recent password sign-in.

| What | Call |
|---|---|
| Read switches, addresses and keys | `GET /api/v1/settings/old-app-api` |
| Turn it on | `PUT /api/v1/settings/old-app-api` with `{"enabled": true}` |
| Let apps delete files | `PUT /api/v1/settings/old-app-api` with `{"delete_files": true}` |
| Make the Sonarr-side key | `POST /api/v1/settings/old-app-api/keys/sonarr` |
| Make the Radarr-side key | `POST /api/v1/settings/old-app-api/keys/radarr` |
| Remove a key | `DELETE /api/v1/settings/old-app-api/keys/sonarr` (or `radarr`) |

</details>

## Related

- [Old-app API clients](../reference/old-app-api-clients/)
- [Old-app API limits](../reference/old-app-api-limits/)
- [Reverse proxy and URL base](../run/url-base/)
