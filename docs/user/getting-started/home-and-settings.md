# First setup, Home and Settings

You just made the owner account and want HolyArrs ready to use. This page
covers the setup steps, what Home shows you, and where to find each setting.

## First setup

Right after you make the owner account, HolyArrs walks you through setup.
Only the owner, or an admin who may change settings, can finish it.

1. **Coming from other apps?** Pick **Bring my apps** to move over from
   Sonarr, Radarr, Lidarr or Overseerr. Pick **Start fresh** to set things up
   by hand. You also say whether family or friends will ask for titles. Yes
   adds a Requests page.
2. **Where's your media?** Turn on what you collect and add its folder. On
   Docker, use the path inside the container, such as `/movies`.
3. **Which app downloads?** Connect your Downloader.
4. **Where should we search?** Link Prowlarr or Jackett, and your sites come
   with it.
5. **You're set.** Open Home, or invite family.

You can skip any step and finish it later in Settings. If you picked
**Bring my apps**, HolyArrs only reads your old apps. Nothing moves until you
say so, and your old apps keep running. See [Migrate](../migrate/move-from-other-apps.md).

Metadata needs no keys. See [Metadata](../metadata/metadata-service.md).

## Home

Home sums up what is happening. The switch at the top shows **Home** or
**System**.

While setup is not done, Home lists what is left:

- Choose a Library folder
- Add a Source
- Add a Downloader
- Add your first title

Each step links to the screen that does it. You can also choose
**Import what you have** to add folders you already own.

Once you are set up, Home shows:

- **Needs attention** with problems to fix, or **All good**.
- Downloads by step: waiting, downloading and importing.
- Your Sources and Downloaders, and how many are working.
- **Coming up** with air dates and releases. The owner gets a link to the
  Calendar.
- Shortcuts to Explore, what is trending, the Calendar and Search.
- How long HolyArrs has been up.

Home updates by itself while downloads, searches and checks run. If a card
says **Could not read**, tap **Try again**.

## Settings

Open Settings from the menu. You only see the screens you may use, and only
for parts you turned on.

- The owner sees every screen.
- An admin or member who may change settings sees the usual screens.
- The owner and admins also see **Move from other apps**, **Parts**, **Users** and **Logs**.
- Everyone sees **My account**.

The Settings home also lists what needs your attention, such as a Source that
does not answer or no recent backup. Each item links to the screen that fixes
it.

### Turn parts on or off

**Settings > Parts** turns Movies, Shows, Music, Subtitles and
Requests on or off. See [Parts and safe mode](../help/parts-and-safe-mode.md).

### Movies and TV defaults

**Settings > Movies and TV defaults** sets what a new movie or show gets when
you add it: its Library folder, its Pick rules, **Keep up to date**, and
**Search when added**. Out of the box, Keep up to date is on for both, and
Search when added is on for movies and off for shows. If a folder or Pick
rules you saved here are removed later, HolyArrs uses the built-in choice and
Settings flags it.

### Appearance

**Settings > Appearance** has the theme (**Automatic**, **Dark** or
**Light**), **Play trailers by themselves**, and **Show Library art on the
sign-in page**. Motion follows your device's Reduce motion setting.

## Troubleshooting

| Problem | Do this |
| --- | --- |
| Setup says only the owner or an admin can finish | Sign in as the owner, or ask them to give you settings access. |
| A Settings screen is missing | Its part is off, or your account may not use it. Ask the owner. |
| Home keeps showing setup steps | Each step must be done: a Library folder, a Source, a Downloader and a title. |
| A folder is not found | Use the path inside the container, not the host path. See [Install](install.md#media-folders). |
