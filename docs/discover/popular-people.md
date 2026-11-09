---
title: New & Popular and People
description: See new and trending titles on streaming services, and browse an actor's or director's work.
level: Start
---

See new releases and what is popular on streaming, and browse everything one actor or director made.

## Find something new

1. Open **New & Popular** in the menu (`http://your-server:6262/popular`).
2. Pick **All**, **TV shows**, **Movies** or **Music** at the top.
3. Scroll the rows. Press a card to open the title, then ask for it or add it.

**Music** shows only when the Music part is on. If Music is turned off while picked, the page goes back to **All**.

The big title at the top changes each visit and says why it is there, such as a new season or a chart place.

Some rows have a chip per streaming service. Pick one to see only that service, or **All**. The pick applies to that row only.

## Browse a person

1. Open a movie or show page.
2. In the cast, press an actor or director.

You should see their page at `http://your-server:6262/people/<id>`. People with the same name stay separate.

| Section | Shows |
|---|---|
| **At a glance** | Known for, birth date and place, death date |
| **Known for** | Their best known work |
| **Filmography** | Every credit by decade. Unreleased movies sit under **Coming soon** and can be followed from the card |
| **Often works with** | People they share the most credits with |
| **About** | Story, credits by decade, **Awards** won and nominated, **Read more on Wikipedia** |

Filter with **Credit role** (**All**, **Actor**, **Director**) and **Sort** (**Popular**, **Newest first**, **Oldest first**). Each card shows where the title stands for you, such as **In your Library**, **Requested** or **Approved, waiting for a copy**.

Viewing a person adds nothing, and HolyArrs does not follow their future work.

## If it does not work

**Symptom:** "New & Popular is getting ready."
**Cause:** rows are still being built from outside lists. **Fix:** wait a few minutes, or use **Explore** meanwhile.

**Symptom:** a New & Popular row is missing.
**Cause:** its list could not be fetched. It comes back on a later visit.

**Symptom:** "No movies match this role."
**Fix:** set **Credit role** to **All**.

**Symptom:** a person page has no photo or story, or does not load.
**Cause:** the metadata server has none, or HolyArrs cannot reach it. **Fix:** try again later.

## For experts

<details class="ha-more"><summary>Where person data comes from</summary>

People, credits and pictures come from the [metadata server](../library/metadata.md). Awards, birth details and the story come from Wikidata and Wikipedia through the same server. HolyArrs does not guess missing names or roles.

</details>

## Related

- [Explore](explore.md)
- [Calendar](calendar.md)
