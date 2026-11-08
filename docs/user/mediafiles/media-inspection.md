# Media inspection

You want to know which codec and audio languages a file really has. HolyArrs reads the part of the file that describes its tracks and records what it finds. It does this when it imports a movie or show, when it takes over a file already on disk, and on rescans.

## What it reads

It reads Matroska (`.mkv`), WebM and MP4 or QuickTime (`.mp4`, `.mov`) files. From them it records:

- **Container**: Matroska, WebM or MP4.
- **Video codec**: H.264, HEVC, AV1, VP9, MPEG-2, Xvid or VC-1, when the file names exactly one of these.
- **Audio languages**: each language found, cut to its base code, so `en-US` counts as `en`.
- **Audio tracks**: how many audio tracks the file has.
- **Unknown audio**: set when a track has no language, has `und`, or has a bad value.
- **Status**: complete, partial or unsupported.

## What it does not do

- It does not play or decode the file. It cannot tell you every frame works.
- It does not guess from the file name or the release name. Anything it cannot read stays **Unknown**.
- It does not use the network or run other programs.

## Where you see it

- Audio languages show in the title page's **Copies** section and decide whether a file meets a Version. See [Original and dubbed audio](audio-versions.md).
- The codec drives the **Video codecs** page. See [Video codecs](library-codecs.md).

## Troubleshooting

**Codec shows Unknown.** The file uses another container, the codec is not in the list above, or the file could not be read.

**A file is reported as damaged.** The track details in the file are cut short or do not add up. Get a fresh copy of the file.

**Music has no inspection.** Music is handled the way it always was.
