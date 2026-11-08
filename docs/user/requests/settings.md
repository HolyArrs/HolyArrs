# Weekly limits and Request rules

You want to control how much people can ask for, and where approved titles go. Open **Settings, Request rules** at `http://your-server:6262/settings/requests`. Only the owner and admins can change these.

## Weekly limits

Each person has three limits over a rolling 7 days: one for movies, one for shows and one for music. They are separate, so asking for a show never uses up a movie ask.

1. Open **Settings, Request rules**.
2. Under **Weekly limits**, set the number for movies, shows and music. The default is 10 each. Use 0 for no limit. The highest is 1000.
3. Save.

To give one person their own limits, open **Settings, Users**, change that person and set their weekly asks. An empty box uses the default.

How asks count:

- A movie counts as one.
- A show counts as one, whatever its seasons.
- An artist or album request counts as one, whatever its albums.
- Asking for more seasons or albums on a waiting request counts as one more.
- Joining someone else's request is free.
- Declined requests do not count.
- People who may approve have no limit.

## Request rules

These apply when an approver leaves the folder or quality on **Default**.

- **Movie folder** and **Movie quality**: where approved movies go, and which quality rules they use.
- **Show folder** and **Show quality**: the same for shows.
- **Music folder** and **Music quality**: the same for artists and albums.
- **Search when approved**: start looking as soon as a request is approved. On by default. Movies search at once, shows at the next feed check.

Left on their defaults, each folder is the first Library folder of that kind, and each quality is the folder's own quality.

## Held back

**Request from Plex watchlists** is held back from this release. It does not show in the app.

## Troubleshooting

**Saving fails on a limit.** Limits must be between 0 and 1000.

**Someone still hits a limit after you raised it.** They may have their own limit on the Users page. Clear it to use the default.
