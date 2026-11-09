---
title: Glossary
description: Every HolyArrs word in one line, with the name older apps use for the same thing.
level: Start
sidebar: { order: 4 }
---

One line per word. Where older apps such as Sonarr and Radarr use another name, the last column gives it.

## HolyArrs words

| Word | Meaning | Older apps call it |
|---|---|---|
| **Adopted** | A file found on disk that you confirmed HolyArrs may manage. | |
| **Alert** | An outgoing message, such as a webhook, Discord or email. | Connect, notification |
| **Approve** | An owner or admin says yes to a request, which adds the title and starts searching. | |
| **Choose a release** | Picking one download by hand from the search results. | Interactive search |
| **Download** | One job in a downloader. | Queue item |
| **Downloader** | The app that downloads for HolyArrs, such as qBittorrent or SABnzbd. | Download client |
| **Episode order** | Which numbering a show follows: aired, absolute, DVD or daily. | |
| **Event** | One row of History. | History item |
| **Find a better copy** | Searching for a higher-quality version of something you have. | |
| **Follow** | Your own choice to see a title's dates on your calendar. Separate from Keep up to date. | |
| **Good enough** | The point where HolyArrs stops looking for a better copy. | |
| **Held** | A release waiting out its wait before it is grabbed. | Pending, delayed |
| **Import** | Placing finished files into the Library, with a plan you can preview. | Manual import |
| **Job** | Background work stored in the database. | Command, task |
| **Keep up to date** | The switch that tells HolyArrs to watch a title for new episodes, releases or better copies. | |
| **Library** | Everything HolyArrs manages: movies, shows and music. | |
| **Library folder** | A folder where HolyArrs keeps finished movies, shows or music. | Root folder |
| **Limit** | How many requests a member may make in a time window, such as 5 movies a week. | Request quota |
| **Linked apps** | The other apps HolyArrs talks to: downloaders, sources and media servers. | |
| **Needs a decision** | An import waiting for a person to approve or discard its plan. | |
| **Never-add list** | Titles outside lists must never add. | Import list exclusions |
| **Outside list** | A feed of titles to add automatically. | Import list |
| **Owner, Admin, Member** | The 3 roles. Owner can do everything; Admin runs the library and approves requests; Member asks for titles. | |
| **Part** | One of 5 pieces you can turn on or off: Movies, TV shows, Music, Subtitles, Requests. | |
| **Pass mark** | The lowest subtitle match score HolyArrs accepts. | Minimum score |
| **Pick rules** | The one rule set that judges releases: quality steps, traits and points, size, wait, language. | Quality profile, custom formats, release and delay profiles |
| **Pressing** | One edition of an album. | Release (album) |
| **Release** | One search result that could be downloaded. | |
| **Release gate** | When a movie counts as out: announced, in cinemas or released. | Minimum availability |
| **Request** | A person's ask for a movie, show, seasons or music, approved or turned down by an owner or admin. | |
| **Requests** | The page where people see their requests and admins approve them. | |
| **Safe mode** | A start that runs only the basics, so you can fix a broken part from the screen. | |
| **Show** | A TV programme with seasons and episodes. | |
| **Source** | A place HolyArrs searches for releases. | Indexer |
| **Stage** | A kind of background work with its own health state, such as search, import or alerts. | |
| **Subtitle site** | A place HolyArrs searches for subtitles. | Provider |
| **Title** | A movie, show or artist in the Library. | |
| **Trait** | A named test inside pick rules, such as words in the name, codec or group. | Custom format |
| **Turned down** | A release HolyArrs will never try again for that item. | Blocklisted |
| **Version** | One wanted copy of a title, such as HD or 4K. A title can have more than one. | Second instance |

## Setup words

| Word | Meaning |
|---|---|
| **Hardlink** | A second name for the same file. Takes no extra space and appears instantly. Works only within one filesystem. |
| **Media server** | The app you watch from, such as Plex, Jellyfin or Emby. HolyArrs talks to it. |
| **Mount** | A folder on your host that the container sees at a path, such as `/volume1/media/movies:/movies`. |
| **PUID and PGID** | The user and group numbers HolyArrs runs as. They must own your media folders. |
| **Reverse proxy** | The web server in front of your apps that sends each address to the right container. |
| **URL base** | The path HolyArrs lives under behind a proxy, such as `/holyarrs`. |
| **UMASK** | Which rights new files get. `022` lets others read but not write. |

## Related

- [FAQ](../help/faq/)
- [Moving from other apps](../migrate/index.md)
