# Add Sources, and link Jackett or Prowlarr

You want HolyArrs to find Releases for your movies, shows and music. For that it needs Sources. A Source is a feed HolyArrs asks for Releases.

HolyArrs reads three kinds of feed itself:

- **Torznab**, for torrents. Jackett and Prowlarr give you these.
- **Newznab**, for Usenet.
- **RSS**, which only gives the newest Releases. You can't search it.

Many torrent sites have no feed of their own. For those, run Jackett or Prowlarr next to HolyArrs. They sign in to the sites for you and give HolyArrs one feed per site.

## Add a Source by hand

1. Open **Settings**, then **Sources**.
2. Press **Add a Source**.
3. Give it a name and pick the **Kind of feed**.
4. Paste the **Address**. For Torznab and Newznab, paste the **Key** too.
5. Press **Save**. HolyArrs tests the Source right away and shows the result under it.

You can press **Test** on any Source later to check it again.

## Link Jackett or Prowlarr

This brings in every site you set up in Jackett or Prowlarr in one go. On **Settings > Sources**, find the **Prowlarr or Jackett** card.

1. Press **Link**.
2. Pick **Jackett** or **Prowlarr**.
3. Type the **Address**. For Jackett, include the base path if you use one. The usual ports are 9117 for Jackett and 9696 for Prowlarr.
4. Paste the **API key**. Jackett shows it on its main page. Prowlarr has it under **Settings > General**.
5. Press **Test** to see what HolyArrs finds, then press **Link**.

### What happens with Jackett

HolyArrs adds one Torznab Source per site in Jackett, with the categories that site offers. The card tells you how many it added.

Every 6 hours, and when you press **Sync**, HolyArrs keeps the Sources in step with Jackett:

- A new site in Jackett becomes a new Source.
- A site you remove in Jackett loses its Source.
- A changed address, key or category follows along.

Everything else on a Source stays as you set it, such as its name, priority, use and limits. You can change how often the check runs in your schedule settings, from every hour to once a week.

### What happens with Prowlarr

You need a HolyArrs password before you can link Prowlarr. The card links you to your account page if you don't have one yet.

HolyArrs adds itself to Prowlarr as an app. It does this once for TV shows and once for movies, only for the parts you have turned on. Prowlarr then sends its sites to HolyArrs and keeps them up to date. The Sources arrive within a minute.

The card shows the address Prowlarr uses to reach HolyArrs, such as `http://your-server:6262`. Press **Change** if Prowlarr can't reach HolyArrs at that address, for example when they run on different machines.

HolyArrs needs a key for each side, TV shows and movies. If a side already has a key, you get two choices:

- Tick **Replace keys**. HolyArrs makes new keys. Any other app that used the old key, like Seerr, needs the new one.
- Or type the key you already have.

### Unlink

Press **Unlink** on the card. You choose what happens to the Sources:

- **Remove its Sources.** For Prowlarr, this also takes HolyArrs out of Prowlarr's app list.
- **Unlink here only. Sources stay.** The Sources become ordinary Sources you manage yourself.

## Settings for each Source

| Setting | What it does |
|---|---|
| Use it for | **Newest list** reads the Source's newest Releases now and then. **Automatic searches** lets HolyArrs ask it when it looks for a title by itself. **Choose a release** lets you ask it from a title's page. An RSS feed can only be used for the newest list. |
| Media | Which kinds of title this Source is asked about: movies, TV shows, music. |
| Priority | 1 to 50. Lower numbers are asked first. When two Releases are equally good, the one from the lower number wins. New Sources start at 25. |
| Fewest seeders | Torrents with fewer seeders are skipped. New Sources start at 1. |
| Tags | Only titles with a matching tag use this Source. Leave it empty to use it for all titles. |
| Limits | **Most searches per window**, **Window (hours)**, **Least time between calls (ms)** and **Give up after (seconds)**. 0 means no limit. |
| Proxy | Sends every call to this Source through an HTTP or SOCKS5 proxy, such as `socks5://name@host:1080` or `http://host:3128`. |
| Reach the local network | Off by default. Turn it on when the Source runs on this machine or your home network, like Jackett or Prowlarr at home. |
| Proxy may be on the local network | The same, for the proxy. |

Keys and proxy passwords are stored encrypted and always shown hidden. If you move a Source to a different host, HolyArrs asks you to type the key again. That way a key is never sent to a new host without you knowing.

Turning on the local network never lets a Source reach cloud metadata addresses.

## Check a Source's health

Each Source shows a short state:

- **Working**: it answered last time.
- **Unreachable**: it did not answer, or answered with something HolyArrs could not read.
- **Key turned down** or **Key refused**: it did not accept the key.
- **Slowed down**: it asked HolyArrs to wait.
- **Blocked**: the site has a bot check, such as Cloudflare. HolyArrs can't read it directly. Use Jackett or Prowlarr for that site.
- **Paused**: you paused it. HolyArrs does not use it until you press **Resume**.

When a Source fails, HolyArrs waits and tries again by itself. It waits one minute at first, then twice as long each time, up to six hours. The Source shows when the next try is. Press **Try now** to try at once.

Press **Health** on a Source to see more:

- its state and last answer,
- its average response time,
- calls, failures and picked Releases per day, for 7, 14, 30 or 90 days,
- its recent calls.

HolyArrs keeps calls that found something, failed or recovered for 90 days. Quiet checks that found nothing new are kept only while HolyArrs runs, so an idle server doesn't write to disk all the time. They are gone after a restart.

## Troubleshooting

| What you see | What to do |
|---|---|
| The key was turned down | Copy the key again from Jackett, Prowlarr or the site, and save it. |
| The address is on the local network | Turn on **Reach the local network**. If it's the proxy, turn on **Proxy may be on the local network**. |
| HolyArrs could not reach the Source | Check that Jackett, Prowlarr or the site is running, and that the address and port are right. |
| The answer is not a feed HolyArrs can read | The address is not a Torznab or Newznab feed. Copy it again. |
| The Source asked HolyArrs to slow down | Wait, or raise **Least time between calls** under **Limits**. |
| The site has a bot check | Add the site to Jackett or Prowlarr and link it. |
| Prowlarr won't link | Set a HolyArrs password first. Check that Prowlarr can reach the HolyArrs address on the card. |
| Another app stopped working after linking Prowlarr | You replaced a key. Give that app the new key. |

See also [Find a title in the Library](library-search.md).
