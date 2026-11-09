
// Chooses which art a poster card shows: backdrop with logo, or backdrop with the name.

export type CardVariant = 'titled' | 'logo' | 'name';

export interface CardArtPick {
	art?: string | null;
	logo?: string | null;
	hasTitle?: boolean;
}

export interface CardTemplate {
	variant: CardVariant;
	image: string | null;
	logo: string | null;
}

// Returns nothing when the server sent no art, so callers keep their own fallback.
export function pickFrom(d: { card_art?: string; card_logo?: string; art_has_title?: boolean } | null | undefined): CardArtPick | undefined {
	if (!d || (!d.card_art && !d.card_logo)) return undefined;
	return { art: d.card_art, logo: d.card_logo || null, hasTitle: !!d.art_has_title };
}

// Art that already has the title baked in is skipped as a backdrop to avoid doubled text.
export function cardTemplate(
	pick: CardArtPick | undefined,
	known: { backdrop?: string | null; logo?: string | null },
	failed: { art?: boolean; logo?: boolean } = {}
): CardTemplate {
	const image = known.backdrop || (pick?.hasTitle ? null : pick?.art) || null;
	const base = failed.art ? null : image;
	const logo = failed.logo ? null : pick?.logo || known.logo || null;
	if (logo) return { variant: 'logo', image: base, logo };
	return { variant: 'name', image: base, logo: null };
}
