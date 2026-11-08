// Explore's category strip under the type switch.
import { type ExploreType } from './explore-type.ts';
import { type BrowseTile } from './search-browse.ts';
import { fromStrip, shownBatcher, stripKey, stripPath, stripRowNames, type StripAnswer } from './category-strip.ts';
import { api } from '#lib/core/api.ts';

export function createExploreStrip(ctx: any) {
	let strip = $state<(BrowseTile & { section: string })[]>([]);
	let stripFor = '';
	async function loadStrip(t: ExploreType) {
		if (t === 'music' || stripFor === t) return;
		stripFor = t;
		strip = [];
		try {
			// Picked for this member on the server from the Browse sections it has ready; the rows below are passed so it skips what they show.
			const a = await api.get<StripAnswer>(stripPath(t, stripRowNames(ctx.head), ctx.viewerZone), ctx.gone.signal);
			if (stripFor !== t) return;
			strip = fromStrip(a.tiles ?? []);
			stripShown.add(strip.map(stripKey));
		} catch {
			if (stripFor === t) stripFor = '';
		}
	}
	$effect(() => {
		const t = ctx.type;
		if (!ctx.headLoaded || ctx.genreOn || t === 'music') return;
		const id = setTimeout(() => void loadStrip(t), 200);
		return () => clearTimeout(id);
	});
	// The strip learns from its own counts: the tiles shown (batched) and the one opened.
	const stripShown = shownBatcher((categories) => void api.post('/explore/strip/counts', { event: 'shown', categories }).catch(() => {}));
	function stripOpened(t: BrowseTile) {
		stripShown.flush();
		void api.post('/explore/strip/counts', { event: 'open', categories: [stripKey(t)] }).catch(() => {});
	}

	// The genre banner: the words on the left, the picks in a row on the right.

	return {
		get stripShown() {
			return stripShown;
		},
		get strip() {
			return strip;
		},
		set strip(v) {
			strip = v;
		},
		stripOpened,
	};
}
