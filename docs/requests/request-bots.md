---
title: Request bots
description: Let chat bots made for Seerr or Overseerr, such as Anchorr and Requestrr, search and ask through HolyArrs.
level: Setup
draft: true
---

Chat bots like Anchorr and Requestrr were made to talk to Seerr or Overseerr. HolyArrs can answer them in Seerr's place. People ask in the chat, and the request lands in HolyArrs **Requests**.

Every ask goes through the normal request rules: permissions, weekly limits and age-rating limits.

## Turn it on

It is off by default. While off, every address under `/seerr/` answers 404. Only the owner and admins change it.

1. Open **Settings > API**.
2. On the **Requests from Seerr and Overseerr** card, switch on **Answer request bots**.
   You should see a row named **Add HolyArrs to the bot as Seerr or Overseerr**.
3. Click **Make key**. Making a key asks for your password again if you have not signed in with it recently.
4. Click **Copy** and keep the key somewhere safe. It is shown only once.
   You should see **Key ending in ...**, with your name and **not used yet**.

## Set up the bot

In the bot's Seerr or Overseerr settings, enter:

| Bot field | Value |
|---|---|
| Address | HolyArrs's address and port, such as `http://your-server:6262` |
| URL base | `/seerr`, or what the card shows (under a URL base it is longer, such as `/holyarrs/seerr`) |
| API key | The key you made |

Save in the bot and send a test search.
You should see the card change to **last used** with a time.

## Who the request counts as

| The bot sends | The request counts as |
|---|---|
| No person | The person who made the key |
| A person (HolyArrs user number) | That person, with their own permissions and limits |

Only a key made by the owner or an admin may ask for someone else. The person must have an active account. A key made by anyone else can only ask as its maker.

The key stops working when its maker's account is turned off. The card then says so. Make a new key.

## What works

| Bot action | In HolyArrs |
|---|---|
| Search | Finds movies and shows by name |
| Show a movie or show | Title details |
| Ask for a movie | A movie request |
| Ask for a show | A show request, for chosen seasons or all seasons |
| Ask in 4K | A 4K request, if that person may ask for 4K |
| List requests | The requests that person may see. Approvers see everyone's |
| Approve or decline | Works for people who may approve |
| Delete a request | Follows the same rules as taking back an ask in HolyArrs |
| List people | Approvers see every active person. Others see only themselves |

## What does not work

- Music. Bots built for Seerr have no music, and music requests are left out of the lists.
- Finding titles by browsing, trending or genre pages. Only search by name.
- Seerr settings beyond the server name. A bot that reads others gets no answer.

## Turn it off or change the key

| To | Do |
|---|---|
| Stop all bots | Switch off **Answer request bots**. `/seerr/` answers 404 again |
| Change the key | Click **New key**. The old key stops working at once. Paste the new one into the bot |
| Remove the key | Click **Remove key**. Bots using it stop working |

## If it does not work

| Symptom | Cause | Fix |
|---|---|---|
| The bot gets 404 | **Answer request bots** is off, or the URL base is wrong | Switch it on. Copy the URL base from the card |
| The bot gets 401 | Wrong or old key | Paste the current key, or click **New key** |
| A person cannot ask for someone else | The key was not made by the owner or an admin | Make the key as the owner or an admin |
| A request is refused | The person hit a weekly limit or has no permission to ask | See [Weekly limits and Request rules](settings.md) |
| Search finds no shows or no movies | That part is off, or the title source is down | Check the part in Settings and try again |

## For experts

<details class="ha-more"><summary>How the bot talks to HolyArrs</summary>

- Address: `/seerr/api/v1/...`, the same paths Seerr uses.
- Key: the `apikey` query value, the `X-Api-Key` header, or `Authorization`. Only the first one found counts.
- Person: the `X-Api-User` header, or `userId` in a request body, holds a HolyArrs user number.
- Each call runs as that person, so every HolyArrs rule applies.
- The key works only under `/seerr/`. It is not a HolyArrs API key, and the Sonarr and Radarr keys do not work here.

</details>
