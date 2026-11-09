---
title: Sources
description: Add Torznab, Newznab or RSS Sources, or link Jackett or Prowlarr, so HolyArrs can find Releases.
level: Setup
sidebar: { order: 3 }
---

Give HolyArrs Sources so it can find Releases for your movies, shows and music.

A Source is a feed HolyArrs asks for Releases. It reads 3 kinds itself:

| Kind | For | Search |
|---|---|---|
| Torznab | Torrents (Jackett and Prowlarr give these) | Yes |
| Newznab | Usenet | Yes |
| RSS | Newest Releases only | No |

Most torrent sites have no feed of their own. For those, run Jackett or Prowlarr next to HolyArrs. They sign in to the sites and give HolyArrs one feed per site.

## Link Jackett or Prowlarr

Already run one of them? Linking brings in every site at once.

1. Open **Settings > Sources** and find the **Prowlarr or Jackett** card.
2. Press **Link**, then pick **Jackett** or **Prowlarr**.
3. Type the **Address**. Usual ports: `9117` for Jackett, `9696` for Prowlarr. For Jackett, include the base path if you set one.
4. Paste the **API key**. Jackett shows it on its main page. Prowlarr has it under **Settings > General**.
5. Press **Test**, then **Link**.

   You should see how many Sources were added.

:::caution
Prowlarr needs a HolyArrs password on your account first. The card links to your account page if you have none.
:::

## Add a Source by hand

1. Open **Settings > Sources**.
2. Press **Add a Source**.
3. Type a name and pick the **Kind of feed**.
4. Paste the **Address**. For Torznab and Newznab, paste the **Key** too.
5. Press **Save**. HolyArrs tests it right away.

   You should see **Working** under it.

Press **Test** on any Source to check it again.

## Choices

### Jackett or Prowlarr

| | Jackett | Prowlarr |
|---|---|---|
| Who keeps Sources in step | HolyArrs checks Jackett every 6 hours and on **Sync** | Prowlarr pushes changes to HolyArrs |
| How it shows up | One Torznab Source per site, with that site's categories | HolyArrs is added to Prowlarr as an app, once for shows and once for movies (only parts you turned on) |
| Time to first Sources | Right after **Link** | Within 1 minute |

With Jackett, a new site becomes a new Source, a removed site loses its Source, and address, key and category changes follow along. Name, priority, use and limits stay as you set them. Change the 6-hour check in your schedule settings, from every hour to once a week.

With Prowlarr, the card shows the address Prowlarr uses to reach HolyArrs, such as `http://your-server:6262`. Press **Change** if Prowlarr runs on another machine and cannot reach that address.

HolyArrs needs a key for each side (shows and movies). If a side already has one, either tick **Replace keys** or type the key you have. Replacing breaks any other app that used the old key, such as Seerr, until you give it the new one.

### Unlink

Press **Unlink** on the card, then pick:

- **Remove its Sources.** For Prowlarr, this also takes HolyArrs out of Prowlarr's app list.
- **Unlink here only. Sources stay.** They become ordinary Sources you manage.

## Health states

| State | Meaning |
|---|---|
| **Working** | Answered last time |
| **Unreachable** | No answer, or an answer HolyArrs could not read |
| **Key turned down** or **Key refused** | The key was not accepted |
| **Slowed down** | The Source asked HolyArrs to wait |
| **Blocked** | The site has a bot check, such as Cloudflare. Use Jackett or Prowlarr for it |
| **Paused** | You paused it. Press **Resume** to use it again |

A failing Source is retried by itself: after 1 minute at first, then at double the wait each time, up to 6 hours. The Source shows the next try. **Try now** tries at once.

## If it does not work

| Symptom | Fix |
|---|---|
| The key was turned down | Copy the key again from Jackett, Prowlarr or the site and save it |
| The address is on the local network | Turn on **Reach the local network**. For the proxy, turn on **Proxy may be on the local network** |
| HolyArrs could not reach the Source | Check that Jackett, Prowlarr or the site is running and the address and port are right |
| The answer is not a feed HolyArrs can read | The address is not a Torznab or Newznab feed. Copy it again |
| The Source asked HolyArrs to slow down | Wait, or raise **Least time between calls** under **Limits** |
| The site has a bot check | Add the site to Jackett or Prowlarr and link it |
| Prowlarr will not link | Set a HolyArrs password first. Check Prowlarr can reach the address on the card |
| Another app stopped working after linking Prowlarr | You replaced a key. Give that app the new key |

More in [Troubleshooting](../../help/troubleshooting/).

## For experts

<details class="ha-more"><summary>Settings reference</summary>

| Setting | What it does | Default | Change it when |
|---|---|---|---|
| Use it for | **Newest list** reads new Releases now and then. **Automatic searches** lets HolyArrs ask it on its own. **Choose a release** lets you ask it from a title page. RSS only does the newest list | | A Source is slow or noisy |
| Media | Which kinds of title it is asked about: movies, TV shows, music | | A site carries one kind only |
| Priority | 1 to 50. Lower is asked first and wins ties between equal Releases | 25 | You trust one Source more |
| Fewest seeders | Torrents with fewer seeders are skipped | 1 | You want healthier torrents |
| Tags | Only titles with a matching tag use it. Empty means all titles | Empty | A Source is for some titles only |
| Limits | **Most searches per window**, **Window (hours)**, **Least time between calls (ms)**, **Give up after (seconds)**. 0 is no limit, except give up after: 1 to 300 | 0 | The site has a daily cap or slows you down |
| Proxy | Sends every call through an HTTP or SOCKS5 proxy, such as `socks5://name@host:1080` or `http://host:3128` | Empty | The site needs another route |
| Reach the local network | Lets it call this machine or your home network | Off | Jackett or Prowlarr runs at home |
| Proxy may be on the local network | The same, for the proxy | Off | Your proxy runs at home |

Keys and proxy passwords are stored encrypted and shown hidden. Moving a Source to another host asks for the key again. Local network access never opens cloud metadata addresses.

</details>

<details class="ha-more"><summary>Health page and what is kept</summary>

**Health** on a Source shows its state and last answer, average response time, calls, failures and picked Releases per day over 7, 14, 30 or 90 days, and recent calls.

Calls that found something, failed or recovered are kept 90 days. Quiet checks that found nothing new are kept in memory only, so an idle server does not write to disk all the time. They are gone after a restart.

</details>

## Related

- [Downloaders](../downloaders/)
- [Library search](../library-search/)
- [History](../history/)
