// The wide title card template (docs/design-review/apple/spec.md, card
// system): the title is always on the art, never under it. Three variants
// share one box bottom-left and one scrim, so a row reads as one pattern:
//   titled - the backdrop already carries the title text;
//   logo   - a textless backdrop with the title logo over it;
//   name   - a textless backdrop with the name set in the display font.

export type CardVariant = 'titled' | 'logo' | 'name';

/** The server's pick for a title (card_art, card_logo, art_has_title). */
export interface CardArtPick {
	art?: string | null;
	logo?: string | null;
	hasTitle?: boolean;
}

export interface CardTemplate {
	variant: CardVariant;
	/** The picture to draw (null: the tile behind the name). */
	image: string | null;
	/** The logo to lay over the art (logo variant only). */
	logo: string | null;
}

/** pickFrom reads the server fields off an Explore item or a Home map entry. */
export function pickFrom(d: { card_art?: string; card_logo?: string; art_has_title?: boolean } | null | undefined): CardArtPick | undefined {
	if (!d || (!d.card_art && !d.card_logo)) return undefined;
	return { art: d.card_art, logo: d.card_logo || null, hasTitle: !!d.art_has_title };
}

/**
 * cardTemplate chooses the variant: titled art first, then the logo, then
 * the name. backdrop and logo are what the card knew before the server's
 * pick (Library titles carry a logo); a picture that failed to load drops
 * that layer, so the card falls back to the next variant.
 */
export function cardTemplate(
	pick: CardArtPick | undefined,
	known: { backdrop?: string | null; logo?: string | null },
	failed: { art?: boolean; logo?: boolean } = {}
): CardTemplate {
	// Netflix-style: the card keeps the title's own backdrop (as it always had)
	// and the logo floats bottom-left; a backdrop with the title printed on it
	// is never used, so the name never shows twice.
	const image = known.backdrop || (pick?.hasTitle ? null : pick?.art) || null;
	const base = failed.art ? null : image;
	const logo = failed.logo ? null : pick?.logo || known.logo || null;
	if (logo) return { variant: 'logo', image: base, logo };
	return { variant: 'name', image: base, logo: null };
}
