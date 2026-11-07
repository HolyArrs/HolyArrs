import { withVersion } from '../core/pictures.ts';

// Bumped when artist photos change source, so browsers drop copies they were told to keep.
const artistPhotoVersion = '4';

/** versioned adds a music picture's stored version (pv, kept a year) when an
 * answer carried its record; an artist photo without one keeps the old
 * cache-busting mark. */
export const versioned = (src: string) => {
	const v = withVersion(src);
	if (v !== src || !src.includes('/music-trending/art/artist/')) return v;
	return src + (src.includes('?') ? '&' : '?') + 'v=' + artistPhotoVersion;
};
