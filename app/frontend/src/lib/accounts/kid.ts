// Kid accounts: an age-rating limit on what a person sees and may ask for. One movie rating is chosen; the server maps it onto the show ladder.

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
	hint: `Movies up to ${value} and shows up to ${show}. Titles without a rating and music stay hidden.`
}));

/** Names a limit for lists: "Kid · up to PG", or '' for none. */
export function kidLabel(max: string | undefined): string {
	return max ? `Kid · up to ${max}` : '';
}

/** Whether this person has an age-rating limit. */
export function isKid(me: { max_rating?: string } | null | undefined): boolean {
	return !!me?.max_rating;
}
