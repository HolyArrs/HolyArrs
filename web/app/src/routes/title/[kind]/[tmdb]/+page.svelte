<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { to } from '#lib/paths.ts';
	import { api, message, ApiError } from '#lib/api.ts';
	import { session } from '#lib/session.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { refreshInbox } from '#lib/wishlist.svelte.ts';
	import { seasonWords } from '#lib/shows.ts';
	import { detailPath, isDetailKind, loadDetails, remembered, type TitleDetails } from '#lib/title-details.ts';
	import type { Ask, GateDefaults, LibraryFolder, LookupMovie, LookupShow, PickRulesView, ShowSettings, TitleDefaults, TitleView } from '#lib/types.ts';
	import TitleHero, { richMetadata, runtimeWords, type RichMeta } from '#lib/ui/TitleHero.svelte';
	import ActionButton from '#lib/ui/ActionButton.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import PosterCard from '#lib/ui/PosterCard.svelte';
	import PersonCircle from '#lib/ui/PersonCircle.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import AddMovieDialog from '#lib/AddMovieDialog.svelte';
	import AddShowDialog from '#lib/AddShowDialog.svelte';
	import AskShowDialog from '#lib/AskShowDialog.svelte';
	import '#lib/title-page.css';

	// A movie or show that is not in the Library yet: the same hero as a
	// Library title, with Add (people who manage the Library) or Ask
	// (members). A title that is in the Library opens its own page instead.
	const kind = $derived(page.params.kind ?? '');
	const tmdb = $derived(page.params.tmdb ?? '');
	let details = $state<TitleDetails | null>(null);
	let loading = $state(true);
	let error = $state('');
	let missing = $state(false);

	const manage = $derived(session.can('manage_library'));
	const mayAsk = $derived(session.can('ask') && !manage);

	let folders = $state<LibraryFolder[]>([]);
	let rules = $state<PickRulesView[]>([]);
	let gateDefault = $state('released');
	let showDefaults = $state<ShowSettings | null>(null);
	let titleDefaults = $state<TitleDefaults | null>(null);
	let addMovie: ReturnType<typeof AddMovieDialog> | undefined = $state();
	let addShow: ReturnType<typeof AddShowDialog> | undefined = $state();
	let askShow: ReturnType<typeof AskShowDialog> | undefined = $state();
	let asking = $state(false);
	let loadedSettings = false;

	async function load(k: string, id: string) {
		if (!isDetailKind(k) || !id) {
			missing = true;
			loading = false;
			return;
		}
		const first = remembered(k, id);
		details = first ? { kind: k, tmdb: id, name: '', genres: [], cast: [], similar: [], ...first } : null;
		loading = true;
		error = '';
		missing = false;
		try {
			const d = await loadDetails(api.get, k, id);
			if (k !== kind || id !== tmdb) return;
			if (!d) {
				missing = !details;
				return;
			}
			if (d.titleId) {
				void goto(to(`/library/${d.titleId}`), { replaceState: true });
				return;
			}
			details = d;
		} catch (e) {
			error = message(e);
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		void load(kind, tmdb);
	});

	$effect(() => {
		if (!manage || loadedSettings) return;
		loadedSettings = true;
		api.get<{ library_folders: LibraryFolder[] }>('/library-folders').then((r) => (folders = r.library_folders)).catch(() => {});
		api.get<{ pick_rules: PickRulesView[] }>('/pick-rules').then((r) => (rules = r.pick_rules)).catch(() => {});
		api.get<GateDefaults>('/settings/release-gate').then((r) => (gateDefault = r.release_gate)).catch(() => {});
		api.get<ShowSettings>('/settings/shows').then((r) => (showDefaults = r)).catch(() => {});
		api.get<TitleDefaults>('/settings/title-defaults').then((r) => (titleDefaults = r)).catch(() => {});
	});

	function added(t: TitleView) {
		toasts.push(`${t.name} is in your Library.`, 'ok');
		void goto(to(`/library/${t.id}`));
	}

	async function askMovie(d: TitleDetails) {
		asking = true;
		try {
			const r = await api.post<{ ask: Ask; joined: boolean; already: boolean }>('/asks', { id: `tmdb:${d.tmdb}` });
			details = { ...d, ask: { id: r.ask.id, state: r.ask.state, state_label: r.ask.state_label, mine: true, people: r.ask.people.length } };
			if (r.ask.state === 'approved') toasts.push(`${r.ask.name} is approved; HolyArrs is looking for it.`, 'ok');
			else if (r.joined) toasts.push(`Someone asked for ${r.ask.name} already; you are on the ask too and will hear when it is ready.`, 'ok');
			else toasts.push(`Asked for ${r.ask.name}. You will hear when it is approved.`, 'ok');
			void refreshInbox();
		} catch (err) {
			if (err instanceof ApiError && err.status === 409 && typeof err.body.title_id === 'number') return void goto(to(`/library/${err.body.title_id}`));
			toasts.push(message(err), 'fail');
		} finally {
			asking = false;
		}
	}

	function showAsked(s: LookupShow, r: { ask: Ask; joined: boolean; already: boolean }) {
		if (details) details = { ...details, ask: { id: r.ask.id, state: r.ask.state, state_label: r.ask.state_label, mine: true, people: r.ask.people.length, seasons: r.ask.seasons } };
		const what = `${r.ask.name} (${seasonWords(r.ask.seasons)})`;
		if (r.ask.state === 'approved') toasts.push(`${what} is approved; HolyArrs keeps it up to date.`, 'ok');
		else if (r.joined) toasts.push(`Someone asked for ${what} already; you are on the ask too and will hear when it is ready.`, 'ok');
		else toasts.push(`Asked for ${what}. You will hear when it is approved.`, 'ok');
		void refreshInbox();
	}

	const kindWord = $derived(kind === 'show' ? 'TV shows' : 'Movies');
	// The logo and age rating, when the metadata endpoint has them (cached, shared with loadDetails' answer).
	let rich = $state<RichMeta | null>(null);
	$effect(() => {
		const k = kind, id = tmdb;
		rich = null;
		if (!isDetailKind(k) || !id) return;
		let live = true;
		void richMetadata(api.get, k, id).then((m) => live && (rich = m));
		return () => (live = false);
	});
	const scoreOf = (r?: string) => (r && /\/\s*10$/.test(r) ? Number.parseFloat(r) : undefined);
	const certOf = (r?: string) => (r && !/\/\s*10$/.test(r) ? r : undefined);
	const heroMeta = (d: TitleDetails) => [d.year ? String(d.year) : '', d.kind === 'show' ? (d.network ?? '') : runtimeWords(d.runtime), ...d.genres.slice(0, 3)].filter(Boolean);
	const crumbs = $derived(details ? [{ href: to('/explore'), label: 'Explore' }, { href: to(detailPath(details.kind, details.tmdb)), label: details.year ? `${details.name} (${details.year})` : details.name }] : []);
</script>

<svelte:head><title>{details?.name ? `${details.name} · HolyArrs` : `${kindWord} · HolyArrs`}</title></svelte:head>

{#if missing}
	<EmptyState icon="search" title="We could not find this title" text="It may have been removed from the movie and show database. Try searching for it by name." action={{ label: 'Search', href: to('/search') }} />
{:else if error && !details}
	<LoadError hasData={false} message="Could not load this title. {error}" onretry={() => void load(kind, tmdb)} />
{:else if !details || !details.name}
	<div class="loading" aria-label="Loading the title">
		<Skeleton shape="block" height="420px" />
		<Skeleton lines={3} />
	</div>
{:else}
	{@const d = details}
	<article class="tp" data-testid="title-detail">
		<TitleHero
			title={d.name}
			poster={d.poster}
			backdrop={d.backdrop}
			logo={rich?.logo}
			crumbIcon={d.kind === 'show' ? 'tv' : 'compass'}
			breadcrumb={crumbs}
			meta={heroMeta(d)}
			certification={rich?.certification ?? certOf(d.rating)}
			score={rich?.score ?? scoreOf(d.rating)}
			status={d.neverAdd ? 'turned_down' : d.ask ? (d.ask.state === 'approved' ? 'approved' : 'requested') : undefined}
			statusLabel={d.neverAdd ? 'On the Never-add list' : d.ask ? (d.ask.mine ? 'You asked' : d.ask.state_label) : undefined}
			description={d.overview}
		>
			{#snippet actions()}
				{#if manage && !d.neverAdd}
					{#if d.kind === 'movie' && d.movie}
						<ActionButton icon="plus" label="Add {d.name}" caption="Add" primary onclick={() => addMovie?.open(d.movie as LookupMovie)} />
					{:else if d.kind === 'show' && d.show}
						<ActionButton icon="plus" label="Add {d.name}" caption="Add" primary onclick={() => addShow?.open(d.show as LookupShow)} />
					{/if}
				{:else if mayAsk && !d.neverAdd}
					{#if d.kind === 'movie'}
						<ActionButton icon="wish" label={d.ask ? `Ask for ${d.name} too` : `Ask for ${d.name}`} caption={d.ask?.mine ? 'Asked' : d.ask ? 'Ask too' : 'Ask'} primary disabled={asking || !!d.ask?.mine} busy={asking} onclick={() => void askMovie(d)} />
					{:else if d.show}
						<ActionButton icon="wish" label={d.ask ? `Ask for ${d.name} too` : `Ask for ${d.name}`} caption={d.ask ? 'Ask too' : 'Ask'} primary onclick={() => askShow?.open(d.show as LookupShow)} />
					{/if}
				{/if}
				<ActionButton icon="search" label="Search for {d.name}"caption="Search" href={to(`/search?q=${encodeURIComponent(d.name)}${d.kind === 'show' ? '&kind=show' : ''}`)} />
			{/snippet}
		</TitleHero>

		{#if d.cast.length}
			<Shelf title="Cast and crew" item="112px">
				{#each d.cast.slice(0, 20) as p, i (p.name + i)}
					<PersonCircle name={p.name} role={p.role} image={p.image} href={p.id ? to(`/people/${p.id}`) : undefined} />
				{/each}
			</Shelf>
		{/if}

		{#if d.similar.length}
			<Shelf title="Similar">
				{#each d.similar.slice(0, 20) as s (s.kind + s.tmdb)}
					<PosterCard title={s.name} subtitle={s.year ? String(s.year) : undefined} image={s.poster} kind={s.kind} href={to(detailPath(s.kind, s.tmdb))} />
				{/each}
			</Shelf>
		{/if}

		<p class="attribution">This product uses the TMDB API but is not endorsed or certified by TMDB.</p>
	</article>
{/if}

{#if manage}
	<AddMovieDialog bind:this={addMovie} {folders} {rules} {gateDefault} {titleDefaults} onadded={(_m, t) => added(t)} />
	<AddShowDialog bind:this={addShow} {folders} {rules} defaults={showDefaults} {titleDefaults} onadded={(_s, t) => added(t)} />
{/if}
{#if mayAsk}
	<AskShowDialog bind:this={askShow} onasked={showAsked} onrefused={(_s, body) => typeof body.title_id === 'number' && void goto(to(`/library/${body.title_id}`))} />
{/if}

<style>
	.loading { display: flex; flex-direction: column; gap: 16px; }
	.attribution { color: var(--text-muted); font: var(--t-footnote); }
</style>
