# Sources, and Jackett or Prowlarr as a bridge

A **Source** is a place HolyArrs looks for Releases. HolyArrs reads three kinds of feed itself:

- **Torznab** for torrents,
- **Newznab** for Usenet,
- a **plain rss feed**, which only gives its newest Releases and cannot be searched.

Many torrent sites have no Torznab feed of their own. For those, run **Jackett** or **Prowlarr** next to HolyArrs. Both sign in to the sites for you and hand HolyArrs one Torznab feed per site. HolyArrs then treats each of those feeds as an ordinary Source. This page shows how.

## Add a Source

1. Open **Settings**, then **Sources**, and press **Add a Source**.
2. Give it a name and pick the kind of feed.
3. Paste the address and, for Torznab and Newznab, the key.
4. Press **Save Source**. HolyArrs tests the new Source straight away and says the result under it.

Keys, rss passkeys and proxy passwords are kept encrypted and are always shown masked. When you change a Source's address to another host, HolyArrs asks for the key again, so a key is never sent to a new host by itself. The same goes for a new proxy on a Source whose address starts with `http://` (the proxy could read the key), and for the proxy password when the proxy's host or name changes.

### What each setting does

| Setting | What it does |
|---|---|
| Use it for | **Newest list** reads the Source's newest Releases now and then. **Searches on its own** lets HolyArrs ask it when it looks for a title by itself. **Search by hand** lets you ask it from a title's page. An rss feed can only be used for the newest list. |
| Media | Which kinds of title this Source is asked about: movies, TV shows, music. |
| Priority | 1 to 50. Sources are asked in this order, 1 first, and when two Releases are equal the one from the Source with the lower number wins. New Sources get 25. |
| Fewest seeders | Torrents with fewer seeders are passed over. New torrent Sources get 1. |
| Limits | Most searches in a window of hours, the least time between two calls, and how long to wait for an answer. 0 means no cap. |
| Proxy | Sends every call to this Source through an HTTP or SOCKS5 proxy, for example `socks5://name@host:1080`. |
| May reach the local network | Off by default. Sources may only reach addresses on the internet unless you tick it. Tick it for a bridge on this machine or your home network. |
| The proxy may be on the local network | The same rule, for the proxy. Even with a proxy, HolyArrs still checks the Source's own address against the first rule: it looks the name up itself and hands the proxy only the checked address (an HTTP proxy gets a tunnel request, so it must allow CONNECT to the Source's port). A name only the proxy can look up is refused. Local-network permission never allows cloud metadata addresses. |

## How healthy is a Source

Each Source shows its state in plain words:

- **Working**: it answered last time.
- **Unreachable**: it did not answer, answered with something HolyArrs could not read, or asked HolyArrs to slow down.
- **Key turned down**: it refused the key. Check the key, then press **Test**.
- **Paused**: you paused it. HolyArrs does not use it until you press **Resume**.

A Source that fails rests for a while and is tried again on its own: one minute, then twice as long each time, up to six hours. The Source shows when the next try is. **Try again now** ends the rest at once, and a working **Test** does too.

Press **Health** on a Source to see its state, its last answer, its average response time, and its calls, failures and picked Releases per day for up to 90 days, plus its last 20 calls. Calls that find work, fail or recover a Source are kept on disk for 90 days. Successful scheduled checks that find nothing new are kept in memory so an idle app does not write to disk. Those checks appear in Health while the app runs and are forgotten when it restarts. The daily totals cover up to 90 days; the rolling last-day count keeps up to 2,048 quiet checks per Source, enough for the fastest supported schedule.

## Connect Jackett or Prowlarr in one step

On **Settings > Sources**, the card **Connect Jackett or Prowlarr** brings every site you set up there in as Sources.

1. Press **Connect Jackett or Prowlarr**, then **Look for them**. HolyArrs looks on this machine, on the Docker host and at the usual Docker names (`jackett`, `prowlarr`) on ports 9117 and 9696. It looks nowhere else and never scans your network. Pick one it found, or type the address.
2. Paste the key. Jackett shows its key at the top of its page; Prowlarr has it under **Settings > General**. The key is kept encrypted.
3. Press **Test** to see what HolyArrs found, then **Connect**.

**Jackett.** HolyArrs adds one Torznab Source per site you set up in Jackett, with the categories the site offers. Every 6 hours, and when you press **Check now**, it keeps them in step: a new site becomes a Source, a site you remove in Jackett takes its Source away, and an address or category change follows. A Source's own name, priority, use and limits are yours and stay as you set them. A Source you remove here stays away; connecting the same Jackett again brings every site back. When Jackett's address is on this machine or your home network, its Sources may reach your home network (Jackett serves their feeds from there), and the card says so; a Jackett on a public address gets ordinary Sources that never reach a local address. **Disconnect** asks whether to remove its Sources or keep them as ordinary ones.

**Prowlarr.** HolyArrs adds itself to Prowlarr as an app, once for TV shows and once for movies (only for the parts you have on), and asks Prowlarr to send its sites. Prowlarr then keeps them up to date through the old-app API (`docs/old-app-api.md`). The setup turns the old-app API on if it is off and makes a key for each side that has none. If a side already has a key, it asks first, because a new key replaces the old one and an app using it (such as Seerr) must get the new one too; or type the key you have. Give the address Prowlarr reaches HolyArrs at, such as `http://192.168.1.10:18730`. The card starts with the address you opened HolyArrs at. A key the setup makes is shown once, right after connecting, so an app that used a replaced one can get it. **Disconnect** takes HolyArrs out of Prowlarr's apps and removes the Sources Prowlarr wrote (or only forgets the connection here, keeping both). Connecting the same Jackett or Prowlarr again takes its new key, and for Prowlarr the new address HolyArrs is reached at.

A site behind a bot check (Cloudflare, DDoS-Guard) cannot be read by HolyArrs directly. Its Source says so and links to this card.

You can still add a bridge site by hand, as below.

## Use Jackett as a bridge

1. Run Jackett next to HolyArrs and add the site in Jackett, signing in there with your own account on that site.
2. In Jackett, press **Copy Torznab Feed** on the site's row. The address looks like `http://jackett-host:9117/api/v2.0/indexers/<site>/results/torznab/`.
3. Copy the Jackett key from the top of the Jackett page.
4. In HolyArrs, add a Source: kind **Torznab**, the copied address, the Jackett key.
5. If Jackett runs on this machine or your home network, tick **May reach the local network**.
6. Save. The test under the Source should say "The Source answered."

Add one Source per site you want, each with its own priority.

## Use Prowlarr as a bridge

1. Run Prowlarr next to HolyArrs and add the site in Prowlarr.
2. In Prowlarr, open the site and note its number. Its Torznab address is `http://prowlarr-host:9696/<number>/api` (a Usenet site uses the same address as a Newznab feed).
3. Copy the Prowlarr key from Prowlarr's general settings.
4. In HolyArrs, add a Source: kind **Torznab** (or **Newznab** for Usenet), that address, the Prowlarr key.
5. Tick **May reach the local network** if Prowlarr runs at home, then save.

Do not also connect Prowlarr to HolyArrs in Prowlarr's own app list: HolyArrs reads Prowlarr's feeds, Prowlarr does not manage HolyArrs.

## When the test fails

| What the test says | What to do |
|---|---|
| The Source turned down the key | Copy the key again from Jackett or Prowlarr and save it. |
| The address is on the local network | Tick **May reach the local network**, or the proxy box if it is the proxy. |
| HolyArrs could not reach the Source | Check the bridge is running and the address and port are right. |
| The Source answered, but not with a feed HolyArrs can read | The address is not a Torznab feed; copy it again from the bridge. |
| The Source asked HolyArrs to slow down | Wait, or raise the least time between calls under **Limits**. |

## Why a bridge, and not site files inside HolyArrs

For now HolyArrs reads only Torznab, Newznab and rss itself. Reading the community's per-site files directly was weighed and left for after version 1 (decision 0005), so the bridge is the way to reach sites without a feed.
