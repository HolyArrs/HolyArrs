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
	if (!d || !d.card_art) return undefined;
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
	const image = pick?.art || known.backdrop || null;
	if (pick?.hasTitle && pick.art && !failed.art) return { variant: 'titled', image: pick.art, logo: null };
	const shown = failed.art ? null : image;
	// A titled picture that failed: the known textless backdrop, if different.
	const base = failed.art && pick?.art && known.backdrop && known.backdrop !== pick.art ? known.backdrop : shown;
	const logo = failed.logo ? null : pick?.logo || known.logo || null;
	if (logo) return { variant: 'logo', image: base, logo };
	return { variant: 'name', image: base, logo: null };
}
