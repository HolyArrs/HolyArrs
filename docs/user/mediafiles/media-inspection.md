# Media header inspection

HolyArrs reads a bounded set of Matroska, WebM, MP4 and QuickTime container headers to identify the video codec and audio track languages. It does not decode media samples, run another process, use the network or guess facts from a filename.

`internal/mediainfo.Inspect` accepts only a regular file. `InspectReaderAt` supports bounded synthetic fixtures and callers that already own a random-access reader. Both return:

- `Container`: `matroska`, `webm` or `mp4` when recognized.
- `VideoCodec`: `h264`, `hevc`, `av1`, `vp9`, `mpeg2`, `xvid` or `vc1` when the inspected header identifies one unambiguous supported codec.
- `AudioLanguages`: sorted, unique lowercase base language codes. Region and script variants reduce to the same matching code used by Version language requirements.
- `AudioTracks`: the actual number of audio tracks found in the inspected headers.
- `AudioUnknown`: true when at least one audio track has a missing, `und` or invalid language value.
- `Status`: `complete`, `partial` or `unsupported`.

An unsupported codec remains unknown. An unsupported container returns `unsupported`. Recognized malformed input returns an error. Unknown facts never fall back to release names or file names.

The reader caps element count, nesting depth, tracks and inspected string lengths. It validates every declared offset and length against the parent container and file size. Large media-data payloads are skipped by offset. It rejects truncated headers, integer overflow, malformed variable-length integers and non-regular filesystem inputs. This is metadata inspection, not a deep corruption scan.

## Format references

- Matroska container elements, track types, codec IDs and language precedence: <https://www.rfc-editor.org/rfc/rfc9559.html>
- QuickTime/ISO media atom structure and sizes: <https://developer.apple.com/documentation/quicktime-file-format/atoms>
- Track handler declaration: <https://developer.apple.com/documentation/quicktime-file-format/handler_reference_atom>
- Media language field and packed language codes: <https://developer.apple.com/documentation/quicktime-file-format/media_header_atom/language> and <https://developer.apple.com/documentation/quicktime-file-format/language_code_values>
- Extended language tags: <https://developer.apple.com/documentation/quicktime-file-format/extended_language_tag_atom/language_tag_string>
- Sample-description layout: <https://developer.apple.com/documentation/quicktime-file-format/sample_description_atom>
- Registered ISO media sample-entry codec codes: <https://mp4ra.org/registered-types/codecs>
