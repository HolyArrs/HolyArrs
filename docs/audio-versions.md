# Original and dubbed audio

Movie and show Versions can require audio languages using two- or three-letter language codes. Common equivalent codes are normalized, such as `eng` to `en` and `fra` to `fr`. Enter several codes separated by commas or spaces. Every requested language must appear in inspected audio tracks. A Version without requirements retains the existing quality-only behavior.

The title's original language is metadata about the work. It is shown separately from the languages actually found in a file. Release names, subtitles and language claims are not proof of an audio track. A multilingual file may satisfy several language requirements, but importing it fills only the Version selected for that download. HolyArrs does not duplicate files or fill another Version automatically.

Bounded container-header inspection records codec, audio tracks, known languages and inspection status for movie and show imports, adoption and rescans. Unsupported or unreadable containers stay Unknown. Unlabelled tracks stay unknown and cannot supply a missing requested language. The inspector does not decode media or prove that every frame is playable. Music's existing handling is unchanged.

An import whose inspected audio cannot meet its Version waits for a decision without replacing the current copy. Review the actual tracks and requirements before choosing another release or changing the Version. Protection continues to apply. Rescans preserve checked editions and protection, and skip unchanged files using size and modification time.

Keep original and dubbed copies as separate Versions with separate folders and requirements. Their quality settings and upgrade progress remain independent. Keeping an existing copy while adding another Version is an explicit action; no automatic duplication is introduced.

## Editions checked by hand: not in the app for now

HolyArrs reads a copy's edition (Theatrical, Director's Cut, Extended and so on) from its file name. Correcting that edition by hand, so a file counts toward the right cut when you keep more than one, is left out of the title page for now. The Copies list still shows an edition when the file name gives one, and editions checked before stay in place.

Correcting an edition by hand will come back in a later release, in a simpler form.
