# Original and dubbed audio

You want one copy of a movie or show in its original language and another dubbed. You do this with Versions. Each Version can ask for audio languages, and HolyArrs checks the real audio tracks in the file before the file counts.

## Keep an original and a dubbed copy

1. Open the title page.
2. Add a Version for each copy you want, such as "Original" and "German".
3. For each Version, enter the audio languages it needs. Use two or three letter codes, such as `en` or `deu`. Separate several codes with commas or spaces.
4. Give each Version its own Library folder and Pick rules.

Each Version keeps its own quality settings and its own upgrades. HolyArrs never copies a file into a second Version on its own.

## How languages are matched

- Codes for the same language count as the same. `eng` matches `en`, and `fra` matches `fr`.
- Every language you list must be in the file's audio tracks.
- A Version with no languages listed only looks at quality.
- The title's original language is shown on its own. It describes the movie or show and says nothing about which tracks a file has.
- Release names, subtitles and language tags in a name do not count. Only tracks HolyArrs finds in the file count. See [Media inspection](media-inspection.md).
- One file with several languages can meet several Versions. An import still fills only the Version the download was for.

These checks apply to movies and shows. Music works as it always has.

## The Copies section

The title page has a **Copies** section with one group per Version. Each group shows the file, the audio HolyArrs found, the edition and whether the file is missing. From there you can:

- **Protect this copy**, so better copies never replace it. See [Protected copies](protected-copies.md).
- **Keep this copy and add another Version**, such as a 4K beside the copy you have.
- **Use an existing video** that already sits in that Version's folder.

## Troubleshooting

**An import is waiting for a decision.** The file's audio cannot meet the Version's languages. The copy you have stays in place. Look at the tracks found and the languages asked for, then pick another release or change the Version.

**Audio shows as unknown.** HolyArrs could not read the file's tracks, or a track has no language label. An unlabelled track cannot stand in for a missing language.

**A rescan changed nothing.** Rescans skip files whose size and change time are the same. Editions and protection stay as they were.
