# First run

You just started HolyArrs and want it ready to use. The first time you open it, a short setup walks you through it. Nobody can use the app until the owner finishes it.

## Make the owner

1. Open `http://your-server:6262` in your browser.
2. Pick an owner name and a password. The password needs at least 8 characters.
3. Sign in. Setup starts right away.

There is no default password and no setup code. Whoever opens a fresh HolyArrs first becomes the owner. Do this right after the first start.

## Answer the setup steps

1. **Coming from other apps?** Pick **Bring my apps** to copy your shows, movies, folders and settings from Sonarr, Radarr, Lidarr, Overseerr and the others. HolyArrs only reads them. Your media files stay where they are, and your old apps keep running. Pick **Start fresh** to set things up by hand. See [Move from other apps](../migrate/move-from-other-apps.md).
2. **Where's your media?** Turn on what you collect (movies, TV shows, music) and add a folder for each. On Docker, use the path inside the container, such as `/movies`, `/tv` or `/music`.
3. **Which app downloads?** Connect your downloader: qBittorrent, SABnzbd, Transmission, NZBGet or Deluge. Enter its address, user name, and password or API key. See [Downloaders](../downloads/downloaders.md).
4. **Where should we search?** Link Prowlarr or Jackett, and your sites come with it. See [Sources](../sources/sources-bridge.md).
5. **You're set.** Press **Open Home** to start, or **Invite family** to turn on Requests and add users.

If you brought your apps, HolyArrs only asks for a downloader or a source when the move didn't bring one.

## Settings explained

- **Skip for now.** The folder, downloader and source steps each have it. Anything you skip shows on the last screen as **Not set**. Tap it to open the right page in Settings and finish it later.
- **Password length.** The 8 character minimum is the default. You can change it under **Settings > Sign-in and security**, from 4 up to 64.
- **Back and Forward.** Your browser's buttons move between steps, and a reload keeps your place.
- **Staying signed in.** The sign-in you used to make the owner stays fresh for up to 24 hours while setup runs, so you are not asked to sign in again halfway.
- **Coming back later.** Open `/welcome` again any time to change what you use: movies, TV shows, music, subtitles, and whether family or friends ask for titles. Saying yes adds a Requests page.

## Troubleshooting

**Someone else became the owner first.** Run this on the server to take it back:

```
docker exec -i holyarrs /holyarrs reset-owner
```

After that, everyone has to sign in again. See [Users and roles](../accounts/users-and-roles.md).

**It says only the owner or an admin can finish setup.** You signed in as someone else. Sign in as the owner, or as an admin with Settings access.

**A folder is refused.** Each folder can be used once, and it can't sit inside another folder you already added. Pick a different path.

**HolyArrs isn't answering.** Wait a moment and try again. If it keeps happening, check that the container is running.
