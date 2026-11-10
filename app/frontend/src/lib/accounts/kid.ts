// Kid accounts: an age-rating limit on what a person sees and may ask for. One movie rating is chosen; the server maps it onto the show ladder.

import { t } from '#lib/core/i18n/i18n.svelte.ts';

export interface RatingChoice {
	value: string;
	show: string;
	hint: string;
}

const pairs: [string, string][] = [
	['G', 'TV-G'],
	['PG', 'TV-PG'],
	['PG-13', 'TV-14'],
	['R', 'TV-MA']
];

export const ratingChoices: RatingChoice[] = pairs.map(([value, show]) => ({
	value,
	show,
	get hint() {
		return t('accounts.kid.hint', { value, show });
	}
}));

/** Names a limit for lists: "Kid · up to PG", or '' for none. */
export function kidLabel(max: string | undefined): string {
	return max ? t('accounts.kid.label', { max }) : '';
}

/** Whether this person has an age-rating limit. */
export function isKid(me: { max_rating?: string } | null | undefined): boolean {
	return !!me?.max_rating;
}
