import { t } from '#lib/core/i18n/i18n.svelte.ts';
import { api } from '#lib/core/api.ts';
import type { MenuItem } from '#lib/ui/Menu.svelte';
import type { SheetEpisode } from '#lib/shows/index.ts';
import { orderMenuItems, splitOutside, type EpisodeGroup, type EpisodeGroupList } from './episode-groups.ts';

interface OrderInput {
	id: number | undefined;
	show: boolean;
	manage: boolean;
	order: string;
	chosen: { id: string; name: string } | null;
	orders: [string, string][];
}

// The Library tab's episode order: the show's TMDB orders and the episodes the chosen one leaves out.
export class EpisodeOrder {
	groupList = $state<EpisodeGroupList | null>(null);
	outside = $state<SheetEpisode[] | null>(null);
	outsideOpen = $state(false);
	#groupsFor = 0;
	#outsideFor = 0;
	#in: () => OrderInput;

	constructor(input: () => OrderInput, pick: (body: Record<string, string>) => void) {
		this.#in = input;
		this.#pick = pick;
		// The show's own TMDB orders, read once when the title opens.
		$effect(() => {
			const a = input();
			if (!a.id || !a.show || !a.manage || this.#groupsFor === a.id) return;
			const id = a.id;
			this.#groupsFor = id;
			this.groupList = null;
			void api.get<EpisodeGroupList>(`/titles/${id}/episode-groups`).then((r) => (this.groupList = r), () => (this.groupList = null));
		});
		// Episodes the chosen order leaves out, listed last under their own heading.
		$effect(() => {
			const a = input();
			if (!a.id || !a.chosen || this.#outsideFor === a.id) return;
			const id = a.id;
			this.#outsideFor = id;
			void api.get<{ episodes: SheetEpisode[] }>(`/titles/${id}/episodes?files=1`).then((r) => (this.outside = splitOutside(r.episodes ?? []).outside), () => (this.outside = null));
		});
	}
	#pick: (body: Record<string, string>) => void;

	menu = $derived.by<MenuItem[]>(() => {
		const a = this.#in();
		const counts = (g: EpisodeGroup) => t('discover.titleLibrary.groupCounts', { seasons: g.seasons, episodes: g.episodes });
		return orderMenuItems({
			orders: a.orders,
			order: a.order,
			chosen: a.chosen?.id ?? null,
			groups: this.groupList?.groups ?? [],
			words: { numbering: t('discover.titleLibrary.numbering'), fromTmdb: t('discover.titleLibrary.ordersFromTmdb'), unavailable: t('discover.titleLibrary.orderUnavailable'), counts },
			pick: this.#pick
		});
	});

	hint = $derived.by(() => {
		const c = this.#in().chosen;
		if (!c) return '';
		const g = this.groupList?.groups.find((x) => x.id === c.id);
		return [c.name || t('discover.titleLibrary.orderUnavailable'), g ? t('discover.titleLibrary.groupSeasons', { n: g.seasons }) : ''].filter(Boolean).join(' · ');
	});

	// Drops the left-out list so it loads again for the new order.
	reload() {
		this.outside = null;
		this.#outsideFor = 0;
	}
}
