// Search's artist cards: the Add and Ask dialogs' folders, rules and answers.
import type { Card } from './search-cards.svelte.ts';
import { albumWords } from '#lib/music/index.ts';
import type { LookupArtist, MusicSettings } from '#lib/music/index.ts';
import { toasts } from '#lib/core/toasts.svelte.ts';
import type { Ask, LibraryFolder, PickRulesView, TitleView } from '#lib/core/types.ts';
import { refreshInbox } from '#lib/core/inbox.svelte.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export function createSearchArtists(ctx: { groups: Record<Card['type'], Card[] | null>; artistCard: (a: LookupArtist) => Card }) {
	let folders = $state<LibraryFolder[]>([]);
	let rules = $state<PickRulesView[]>([]);
	let musicDefaults = $state<MusicSettings | null>(null);
	function patchArtist(a: LookupArtist, change: Partial<LookupArtist>) {
		const list = ctx.groups.music;
		if (!list) return;
		const i = list.findIndex((c) => c.artist?.mbid === a.mbid);
		if (i >= 0) list[i] = ctx.artistCard({ ...a, ...change });
	}
	function artistAdded(a: LookupArtist, title: TitleView) {
		patchArtist(a, { title_id: title.id });
		toasts.push(t('discover.searchArtists.added', { name: title.name }), 'ok', { body: t('discover.searchArtists.addedBody') });
	}
	function artistAsked(a: LookupArtist, r: { ask: Ask; joined: boolean; already: boolean }) {
		patchArtist(a, { ask: { id: r.ask.id, state: r.ask.state, state_label: r.ask.state_label, mine: true, people: r.ask.people.length, albums: r.ask.albums } });
		const what = `${r.ask.name} (${albumWords(r.ask.albums)})`;
		if (r.ask.state === 'approved') toasts.push(t('discover.searchArtists.approved', { what }), 'ok', { body: t('discover.searchArtists.keptUpToDate') });
		else if (r.joined) toasts.push(t('discover.searchArtists.alreadyRequested', { what }), 'ok', { body: t('discover.searchArtists.onRequestToo') });
		else toasts.push(t('discover.searchArtists.requested', { what }), 'ok', { body: t('discover.searchArtists.waitingForApproval') });
		void refreshInbox();
	}
	function artistRefused(a: LookupArtist, body: Record<string, unknown>) {
		if (typeof body.title_id === 'number') patchArtist(a, { title_id: body.title_id });
	}

	return {
		get folders() {
			return folders;
		},
		set folders(v) {
			folders = v;
		},
		get rules() {
			return rules;
		},
		set rules(v) {
			rules = v;
		},
		get musicDefaults() {
			return musicDefaults;
		},
		set musicDefaults(v) {
			musicDefaults = v;
		},
		artistAdded,
		artistAsked,
		artistRefused,
	};
}
