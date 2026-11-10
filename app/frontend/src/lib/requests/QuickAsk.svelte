<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { cardAction, cardTmdb, liveCard, type ExploreCard } from '#lib/ui/title-cards.ts';
	import AddMovieDialog from './AddMovieDialog.svelte';
	import AddShowDialog from './AddShowDialog.svelte';
	import AskShowDialog from './AskShowDialog.svelte';
	import AskMovieDialog from './AskMovieDialog.svelte';
	import { loadDetails } from '../metadata/index.ts';
	import { seasonWords } from '../shows/index.ts';
	import { savedDefaults } from '../mediafiles/index.ts';
	import type { GateDefaults } from '../mediafiles/index.ts';
	import type { ShowSettings } from '../shows/index.ts';
	import { goto } from '$app/navigation';
	import { api, message, ApiError } from '../core/api.ts';
	import { session } from '../core/session.svelte.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import { to } from '../core/paths.ts';
	import { refreshInbox } from '../core/inbox.svelte.ts';
	import type { Ask, AskState, LibraryFolder, PickRulesView, TitleView } from '../core/types.ts';
	import type { LookupShow } from '../core/types.ts';

	// The round + on a movie or show card, shared by every page with title cards. It never adds or asks in one click but opens a confirm sheet (seasons for shows; Library folder and Pick rules for people who manage).
	let { onadded }: { onadded?: (title: TitleView) => void } = $props();

	const manage = $derived(session.can('manage_library'));
	const mayAsk = $derived(session.can('ask'));

	let folders = $state<LibraryFolder[]>([]);
	let rules = $state<PickRulesView[]>([]);
	let gateDefault = $state('released');
	let showDefaults = $state<ShowSettings | null>(null);
	let addMovie: ReturnType<typeof AddMovieDialog> | undefined = $state();
	let addShow: ReturnType<typeof AddShowDialog> | undefined = $state();
	let askShow: ReturnType<typeof AskShowDialog> | undefined = $state();
	let askMovie: ReturnType<typeof AskMovieDialog> | undefined = $state();
	/** Cards asked for (with the ask state returned) or found in the Library here, by card key. */
	let askedHere = $state(new Map<string, { ask?: AskState; titleId?: number }>());
	let asking = $state('');
	/** Which card the sheet is for, so the card updates once it is asked or added. */
	let current: ExploreCard | null = null;

	/** The card as it stands after anything asked or added here. */
	export function live(c: ExploreCard): ExploreCard {
		return liveCard(c, askedHere.get(c.key));
	}

	/** Add, Ask or nothing for this card and this person. */
	export function action(c: ExploreCard): 'add' | 'ask' | null {
		return cardTmdb(c) ? cardAction(live(c), { manage, ask: mayAsk }) : null;
	}

	/** The Library folders and Pick rules the add sheets offer, for other add sheets on the page (music). */
	export function addChoices(): { folders: LibraryFolder[]; rules: PickRulesView[] } {
		return { folders, rules };
	}

	/** Whether this card's sheet is loading. */
	export function busy(c: ExploreCard): boolean {
		return asking === c.key;
	}

	export async function open(c: ExploreCard, act: 'add' | 'ask') {
		const tmdb = cardTmdb(c);
		if (!tmdb || asking) return;
		current = c;
		if (act === 'ask' && c.kind === 'movie') return askMovie?.open({ tmdb, name: c.name, year: c.year, ask: c.ask });
		asking = c.key;
		try {
			const d = await loadDetails(api.get, c.kind as 'movie' | 'show', tmdb);
			if (!d) return void toasts.push(t('requests.quick.notFound', { name: c.name }), 'fail', { body: t('requests.quick.trySearch') });
			if (d.titleId) return void toasts.push(t('requests.quick.inLibrary', { name: c.name }), 'ok');
			if (act === 'add' && d.movie) addMovie?.open(d.movie);
			else if (act === 'add' && d.show) addShow?.open(d.show);
			else if (d.show) askShow?.open(d.show);
		} catch (e) {
			toasts.push(t('requests.quick.openFailed'), 'fail', { body: message(e) });
		} finally {
			asking = '';
		}
	}

	/** One tap asks for the whole title (every season of a show) with no sheet; the card then reads as asked. */
	export async function askNow(c: ExploreCard) {
		const tmdb = cardTmdb(c);
		if (!tmdb || asking || (c.kind !== 'movie' && c.kind !== 'show')) return;
		current = c;
		asking = c.key;
		try {
			const r = await api.post<{ ask: Ask; joined: boolean; already: boolean }>('/asks', c.kind === 'movie' ? { id: `tmdb:${tmdb}`, follow: true } : { kind: 'show', id: `tmdb:${tmdb}`, seasons: [], follow: true });
			asked(r, c.name);
		} catch (err) {
			if (err instanceof ApiError && err.status === 409 && typeof err.body.title_id === 'number') {
				refused(err.body);
				const id = err.body.title_id;
				toasts.push(t('requests.quick.inYourLibrary', { name: c.name }), 'info', { action: { label: t('requests.quick.open'), run: () => void goto(to(`/library/${id}`)) } });
				return;
			}
			if (err instanceof ApiError && err.status === 409) refused(err.body);
			toasts.push(t('requests.quick.sendFailed'), 'fail', { body: message(err) });
		} finally {
			asking = '';
		}
	}

	function asked(r: { ask: Ask; joined: boolean }, what: string) {
		if (current) askedHere = new Map([...askedHere, [current.key, { ask: r.ask.state }]]);
		if (r.ask.state === 'approved') toasts.push(t('requests.quick.approved', { what }), 'ok', { body: t('requests.quick.searching') });
		else if (r.joined) toasts.push(t('requests.quick.already', { what }), 'ok', { body: t('requests.quick.onIt') });
		else toasts.push(t('requests.quick.requested', { what }), 'ok', { body: t('requests.quick.waiting') });
		void refreshInbox();
	}

	/** The server says the title is in the Library already (409), so the card says so. */
	function refused(body: Record<string, unknown>) {
		if (current && typeof body.title_id === 'number') askedHere = new Map([...askedHere, [current.key, { titleId: body.title_id }]]);
	}

	function added(title: TitleView) {
		if (current) askedHere = new Map([...askedHere, [current.key, { titleId: title.id }]]);
		toasts.push(t('requests.quick.added', { name: title.name }), 'ok');
		onadded?.(title);
	}

	$effect(() => {
		if (!manage) return;
		api.get<{ library_folders: LibraryFolder[] }>('/library-folders').then((r) => (folders = r.library_folders)).catch(() => {});
		api.get<{ pick_rules: PickRulesView[] }>('/pick-rules').then((r) => (rules = r.pick_rules)).catch(() => {});
		api.get<GateDefaults>('/settings/release-gate').then((r) => (gateDefault = r.release_gate)).catch(() => {});
		api.get<ShowSettings>('/settings/shows').then((r) => (showDefaults = r)).catch(() => {});
		// The add dialogs wait for the saved defaults, fetched once per visit.
		savedDefaults.forget();
		void savedDefaults.load();
	});
</script>

{#if manage}
	<AddMovieDialog bind:this={addMovie} {folders} {rules} {gateDefault} onadded={(_m, title) => added(title)} />
	<AddShowDialog bind:this={addShow} {folders} {rules} defaults={showDefaults} onadded={(_s, title) => added(title)} />
{:else if mayAsk}
	<AskMovieDialog bind:this={askMovie} onasked={(_m, r) => asked(r, r.ask.name)} onrefused={(_m, body) => refused(body)} />
	<AskShowDialog bind:this={askShow} onasked={(_s: LookupShow, r) => asked(r, `${r.ask.name} (${seasonWords(r.ask.seasons)})`)} onrefused={(_s: LookupShow, body: Record<string, unknown>) => refused(body)} />
{/if}
