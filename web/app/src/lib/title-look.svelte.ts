// Where a title page puts its facts line and tagline while Luis compares the
// options (?look=both|band). The choice is kept per browser so every
// title opens with the same option.
export type TitleLook = 'quiet' | 'credits' | 'scores' | 'herostory' | 'side' | 'story' | 'decide' | 'both' | 'band';

export const TITLE_LOOKS: { value: TitleLook; label: string }[] = [
	{ value: 'quiet', label: 'V1 · Quiet' },
	{ value: 'credits', label: 'V2 · Story + credits' },
	{ value: 'scores', label: 'V3 · Scores in hero' },
	{ value: 'herostory', label: 'V4 · Story in hero' },
	{ value: 'side', label: 'V5 · Credits beside' },
	{ value: 'story', label: 'V6 · Story first' },
	{ value: 'decide', label: 'V7 · Split' }
];

const KEY = 'holyarrs.hero-look-3';
const known = (v: string | null): v is TitleLook => v === 'quiet' || v === 'credits' || v === 'herostory' || v === 'side' || v === 'scores' || v === 'story' || v === 'decide' || v === 'both' || v === 'band';

function first(): TitleLook {
	if (typeof window === 'undefined') return 'quiet';
	const q = new URLSearchParams(location.search).get('look');
	if (known(q)) return q;
	try {
		const s = localStorage.getItem(KEY);
		if (known(s)) return s;
	} catch {
		/* No storage: the default option. */
	}
	return 'quiet';
}

export const titleLook = $state({ value: first() });

export function setTitleLook(v: TitleLook) {
	titleLook.value = v;
	try {
		localStorage.setItem(KEY, v);
	} catch {
		/* Kept for this visit only. */
	}
}

// Where the tags sit (?tags=band|row|text|top), kept the same way.
export type TagsAt = 'band' | 'row' | 'text' | 'top';
export const TAG_SPOTS: { value: TagsAt; label: string }[] = [
	{ value: 'band', label: 'T1 · Under story' },
	{ value: 'row', label: 'T2 · In About' }
];
const TAGS_KEY = 'holyarrs.tags-at';
const knownTags = (v: string | null): v is TagsAt => v === 'band' || v === 'row' || v === 'text' || v === 'top';
function firstTags(): TagsAt {
	if (typeof window === 'undefined') return 'row';
	const q = new URLSearchParams(location.search).get('tags');
	if (knownTags(q)) return q;
	try {
		const s = localStorage.getItem(TAGS_KEY);
		if (knownTags(s)) return s;
	} catch {
		/* No storage: the default spot. */
	}
	return 'row';
}
export const tagsAt = $state({ value: firstTags() });
export function setTagsAt(v: TagsAt) {
	tagsAt.value = v;
	try {
		localStorage.setItem(TAGS_KEY, v);
	} catch {
		/* Kept for this visit only. */
	}
}
