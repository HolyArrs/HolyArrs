// Bumped when artist photos change source, so browsers drop copies they were told to keep.
const artistPhotoVersion = '4';

export const versioned = (src: string) => (src.includes('/music-trending/art/artist/') ? src + (src.includes('?') ? '&' : '?') + 'v=' + artistPhotoVersion : src);
