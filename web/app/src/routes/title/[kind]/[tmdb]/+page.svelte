<script lang="ts">
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import { page } from '$app/state';
	import { goto, snapshot } from '$app/navigation';
	import { to } from '#lib/paths.ts';
	import { api, message, ApiError } from '#lib/api.ts';
	import { session } from '#lib/session.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { refreshInbox } from '#lib/wishlist.svelte.ts';
	import { seasonWords } from '#lib/shows.ts';
	import { detailPath, isDetailKind, loadDetails, remembered, type TitleDetails } from '#lib/title-details.ts';
	import { preloadHero } from '#lib/tmdb-image.ts';
	import type { Ask, GateDefaults, LibraryFolder, LookupMovie, LookupShow, PickRulesView, ShowSettings, TitleView } from '#lib/types.ts';
	import TitleHero, { richMetadata, runtimeWords, type RichMeta } from '#lib/ui/TitleHero.svelte';
	import ActionButton from '#lib/ui/ActionButton.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import TitleRows from '#lib/TitleRows.svelte';
	import ShowSeasons from '#lib/ShowSeasons.svelte';
	import FollowButton from '#lib/ui/FollowButton.svelte';
	import type { InfoDetails } from '#lib/title-info.ts';
	import TitleSection from '#lib/ui/TitleSection.svelte';
	import SectionNav from '#lib/ui/SectionNav.svelte';
	import { pageSections } from '#lib/title-page.ts';
	import { tmdbArtName } from '#lib/explore.ts';
	import SeasonFolds from '#lib/SeasonFolds.svelte';
	import TitleLookPicker from '#lib/TitleLookPicker.svelte';
	import { titleLook } from '#lib/title-look.svelte.ts';
	import PersonCircle from '#lib/ui/PersonCircle.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import AddMovieDialog from '#lib/AddMovieDialog.svelte';
	import { savedDefaults } from '#lib/title-defaults.svelte.ts';
	import AddShowDialog from '#lib/AddShowDialog.svelte';
	import AskShowDialog from '#lib/AskShowDialog.svelte';
	import AskMovieDialog from '#lib/AskMovieDialog.svelte';
	import '#lib/title-page.css';
	import { audienceOf, titleStatus } from '#lib/title-status.ts';

	// A movie or show that is not in the Library yet: the same hero as a
	// Library title, with Add (people who manage the Library) or Ask
	// (members). A title that is in the Library opens its own page instead.
	const kind = $derived(page.params.kind ?? '');
	const tmdb = $derived(page.params.tmdb ?? '');
	let details = $state<TitleDetails | null>(null);
	// Back from a person (or any page opened here) lands where the page was:
	// rows below the hero load late, so the scroll is put back as they arrive.
	snapshot<number>({
		capture: () => Math.round(scrollY),
		restore: (y) => {
			if (!y) return;
			const until = Date.now() + 4000;
			const put = () => {
				if (document.documentElement.scrollHeight >= y + innerHeight) return scrollTo({ top: y, behavior: 'instant' });
				if (Date.now() < until) requestAnimationFrame(put);
				else scrollTo({ top: y, behavior: 'instant' });
			};
			put();
			for (const ms of [300, 900]) setTimeout(() => Math.abs(scrollY - y) > 4 && document.documentElement.scrollHeight >= y + innerHeight && scrollTo({ top: y, behavior: 'instant' }), ms);
		}
	});
	let loading = $state(true);
	let error = $state('');
	let missing = $state(false);
	// A Kid account asked for a title above its age-rating limit (403).
	let blocked = $state(false);

	const manage = $derived(session.can('manage_library'));
	const mayAsk = $derived(session.can('ask') && !manage);

	let folders = $state<LibraryFolder[]>([]);
	let rules = $state<PickRulesView[]>([]);
	let gateDefault = $state('released');
	let showDefaults = $state<ShowSettings | null>(null);

	let addMovie: ReturnType<typeof AddMovieDialog> | undefined = $state();
	let addShow: ReturnType<typeof AddShowDialog> | undefined = $state();
	let askShow: ReturnType<typeof AskShowDialog> | undefined = $state();
	// Cast and Similar are one row each; See all opens the whole list.
	let allCast = $state(false);
	let askMovieDialog: ReturnType<typeof AskMovieDialog> | undefined = $state();
	let loadedSettings = false;

	async function load(k: string, id: string) {
		if (!isDetailKind(k) || !id) {
			missing = true;
			loading = false;
			return;
		}
		const first = remembered(k, id);
		// The hero backdrop starts loading before the details answer arrives.
		preloadHero(first?.backdrop);
		details = first ? { kind: k, tmdb: id, name: '', genres: [], cast: [], similar: [], ...first } : null;
		loading = true;
		error = '';
		missing = false;
		blocked = false;
		try {
			const d = await loadDetails(api.get, k, id);
			if (k !== kind || id !== tmdb) return;
			preloadHero(d?.backdrop);
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
			if (e instanceof ApiError && e.status === 403) {
				blocked = true;
				details = null;
			} else error = message(e);
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		void load(kind, tmdb);
		allCast = false;
	});

	$effect(() => {
		if (!manage || loadedSettings) return;
		loadedSettings = true;
		api.get<{ library_folders: LibraryFolder[] }>('/library-folders').then((r) => (folders = r.library_folders)).catch(() => {});
		api.get<{ pick_rules: PickRulesView[] }>('/pick-rules').then((r) => (rules = r.pick_rules)).catch(() => {});
		api.get<GateDefaults>('/settings/release-gate').then((r) => (gateDefault = r.release_gate)).catch(() => {});
		api.get<ShowSettings>('/settings/shows').then((r) => (showDefaults = r)).catch(() => {});
		// The add dialogs wait for the saved defaults; fetched once for this visit.
		savedDefaults.forget();
		void savedDefaults.load();
	});

	function added(t: TitleView) {
		toasts.push(`${t.name} is in your Library.`, 'ok');
		void goto(to(`/library/${t.id}`));
	}

	function movieAsked(r: { ask: Ask; joined: boolean; already: boolean }) {
		if (details) details = { ...details, ask: { id: r.ask.id, state: r.ask.state, state_label: r.ask.state_label, mine: true, people: r.ask.people.length } };
		if (r.ask.state === 'approved') toasts.push(`${r.ask.name} is approved; HolyArrs is looking for it.`, 'ok');
		else if (r.joined) toasts.push(`Someone asked for ${r.ask.name} already; you are on the ask too and will hear when it is ready.`, 'ok');
		else toasts.push(`Asked for ${r.ask.name}. You will hear when it is approved.`, 'ok');
		void refreshInbox();
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
	// One status vocabulary (title-status.ts) for the ask on this title.
	const heroPill = (d: TitleDetails) => titleStatus({ ask: d.ask?.state, mine: d.ask?.mine, neverAdd: d.neverAdd, titleKind: d.kind }, audienceOf(session.can));
	const scoreOf = (r?: string) => (r && /\/\s*10$/.test(r) ? Number.parseFloat(r) : undefined);
	const certOf = (r?: string) => (r && !/\/\s*10$/.test(r) ? r : undefined);
	// The details answer read by the rows (seasons for a show).
	let info = $state<InfoDetails | null>(null);
	const seasons = $derived(info?.seasons ?? []);
	const regular = $derived(seasons.filter((s) => s.number > 0).length);
	const networkOf = (d: TitleDetails) => d.network ?? d.show?.network ?? '';
	// The versions of layout A being compared (title-look.svelte.ts). The hero
	// says what it is (year, length or seasons, genres, age rating, scores);
	// the details below say only what the hero does not.
	const look = $derived(titleLook.value);
	const personHref = (id?: string) => (id ? to(`/people/${id.replace(/^tmdb:/, '')}`) : undefined);
	/** The director (movies) or creators (shows). */
	const makers = $derived((info?.crew ?? []).filter((p) => p.role === (kind === 'show' ? 'creator' : 'director')));
	const madeBy = $derived(kind === 'show' ? 'Created by' : 'Directed by');
	const heroFacts = (d: TitleDetails) =>
		[d.year ? String(d.year) : '', d.kind === 'show' && regular ? `${regular} ${regular === 1 ? 'season' : 'seasons'}` : d.kind === 'movie' ? runtimeWords(d.runtime) : '', ...d.genres.slice(0, d.kind === 'show' ? 2 : 3)].filter(Boolean);
	const langName = (code?: string) => {
		if (!code) return '';
		try {
			return new Intl.DisplayNames(['en'], { type: 'language' }).of(code) ?? code;
		} catch {
			return code;
		}
	};
	const countryName = (code: string) => {
		try {
			return new Intl.DisplayNames(['en'], { type: 'region' }).of(code) ?? code;
		} catch {
			return code;
		}
	};
	const lang = $derived(langName(info?.originalLanguage));
	const countries = $derived((info?.originCountry ?? []).map(countryName));
	const network = $derived(details ? networkOf(details) : '');
	const hasFacts = $derived(!!(network || makers.length || lang || countries.length));
	const hasCard = $derived(hasFacts || !!info?.streaming.length);
	const crumbs = $derived(details ? [{ href: to('/explore'), label: 'Explore' }, { href: to(detailPath(details.kind, details.tmdb)), label: details.year ? `${details.name} (${details.year})` : details.name }] : []);
</script>

<svelte:head><title>{details?.name ? `${details.name} · HolyArrs` : `${kindWord} · HolyArrs`}</title></svelte:head>

{#if blocked}
	<EmptyState icon="shield" title="This title is not available on this account." action={{ label: 'Back to Explore', href: to('/explore') }} />
{:else if missing}
	<EmptyState icon="search" title="We could not find this title" text="It may have been removed from the movie and show database. Try searching for it by name." action={{ label: 'Search', href: to('/search') }} />
{:else if error && !details}
	<LoadError hasData={false} message="Could not load this title. {error}" onretry={() => void load(kind, tmdb)} />
{:else}
<PageLoader loading={!details || !details.name} label="Loading the title" onretry={() => void load(kind, tmdb)}>
{#if details}
	{@const d = details}
	<article class="tp" data-testid="title-detail">
		<TitleHero
			title={d.name}
			kind={d.kind}
			tmdb={d.tmdb}
			poster={d.poster}
			backdrop={d.backdrop}
			backdrops={rich?.backdrops}
			logo={rich?.logo}
			crumbIcon={d.kind === 'show' ? 'tv' : 'compass'}
			breadcrumb={crumbs}
			meta={heroFacts(d)}
			certification={rich?.certification ?? certOf(d.rating)}
			score={rich?.score ?? scoreOf(d.rating)}
			status={d.neverAdd || d.ask ? heroPill(d).kind : undefined}
			statusLabel={d.neverAdd || d.ask ? heroPill(d).label : undefined}
			vtName={tmdbArtName(d.kind, d.tmdb)}
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
						<ActionButton icon="wish" label={d.ask ? `Ask for ${d.name} too` : `Ask for ${d.name}`} caption={d.ask?.mine ? 'Asked' : d.ask ? 'Ask too' : 'Ask'} primary disabled={!!d.ask?.mine} onclick={() => askMovieDialog?.open({ tmdb: d.tmdb, name: d.name, year: d.year, ask: d.ask })} />
					{:else if d.show}
						<ActionButton icon="wish" label={d.ask ? `Ask for ${d.name} too` : `Ask for ${d.name}`} caption={d.ask ? 'Ask too' : 'Ask'} primary onclick={() => askShow?.open(d.show as LookupShow)} />
					{/if}
				{/if}
				{#if d.kind === 'show'}<FollowButton tmdb={d.tmdb} kind="show" />{/if}
			{/snippet}
		</TitleHero>

		<SectionNav items={pageSections(d.kind, { manage: false })} />

		{#snippet castRow()}
			{#if d.cast.length}
				<Shelf title="Cast and crew" item="112px" anchor="title-cast" onseeall={() => (allCast = true)}>
					{#each d.cast.slice(0, 12) as p, i (p.name + i)}
						<PersonCircle name={p.name} role={p.role} image={p.image} href={personHref(p.id)} />
					{/each}
				</Shelf>
			{/if}
		{/snippet}
		{#snippet names(list: { name: string; id?: string }[], max: number)}
			{#each list.slice(0, max) as p, i (p.name + i)}{#if i}{', '}{/if}{#if personHref(p.id)}<a href={personHref(p.id)}>{p.name}</a>{:else}{p.name}{/if}{/each}
		{/snippet}
		{#snippet services()}
			{#if info?.streaming.length}
				<ul class="svc">
					{#each info.streaming as sv (sv.id)}
						<li title={sv.name}>{#if sv.logo}<img src={sv.logo} alt="" width="28" height="28" loading="lazy" />{/if}<span>{sv.name}</span></li>
					{/each}
				</ul>
			{/if}
		{/snippet}
		{#snippet story()}
			{#if d.overview}<p class="lead" data-testid="title-story">{d.overview}</p>{/if}
		{/snippet}
		{#snippet card()}
			<div class="facts" id="title-where">
				{#if hasFacts}
					<dl class="fact-list">
						{#if network}<dt>Network</dt><dd>{network}</dd>{/if}
						{#if makers.length}<dt>{madeBy}</dt><dd>{@render names(makers, 3)}</dd>{/if}
						{#if lang}<dt>Language</dt><dd>{lang}</dd>{/if}
						{#if countries.length}<dt>{countries.length > 1 ? 'Countries' : 'Country'}</dt><dd>{countries.join(', ')}</dd>{/if}
					</dl>
				{/if}
				{#if info?.streaming.length}
					<div class="fact-where"><h2 class="tp-h3">Where to watch</h2>{@render services()}</div>
				{/if}
			</div>
		{/snippet}
		{#snippet folds()}
			{#if d.kind === 'show'}<SeasonFolds tmdb={d.tmdb} {seasons} fallback={d.backdrop} />{/if}
		{/snippet}

		{#if look === 'stack'}
			<!-- A2 · Stacked: the story and where to watch beside a small details card (about the same height), the seasons full width under them. -->
			<div class="tp-body look-a stacked" class:solo={!hasFacts}>
				<div class="tp-main">
					{@render story()}
					{#if info?.streaming.length}<div class="where-row" id="title-where"><span class="lbl">Where to watch</span>{@render services()}</div>{/if}
				</div>
				{#if hasFacts}<aside class="tp-side"><div class="facts"><dl class="fact-list">
					{#if network}<dt>Network</dt><dd>{network}</dd>{/if}
					{#if makers.length}<dt>{madeBy}</dt><dd>{@render names(makers, 3)}</dd>{/if}
					{#if lang}<dt>Language</dt><dd>{lang}</dd>{/if}
					{#if countries.length}<dt>{countries.length > 1 ? 'Countries' : 'Country'}</dt><dd>{countries.join(', ')}</dd>{/if}
				</dl></div></aside>{/if}
			</div>
			{@render folds()}
		{:else if look === 'strip'}
			<!-- A3 · Strip: no side card; the details as one row of small columns under the story, the seasons full width. -->
			<div class="strip">
				{@render story()}
				{#if hasFacts}
					<dl class="inline-facts">
						{#if network}<div><dt>Network</dt><dd>{network}</dd></div>{/if}
						{#if makers.length}<div><dt>{madeBy}</dt><dd>{@render names(makers, 3)}</dd></div>{/if}
						{#if lang}<div><dt>Language</dt><dd>{lang}</dd></div>{/if}
						{#if countries.length}<div><dt>{countries.length > 1 ? 'Countries' : 'Country'}</dt><dd>{countries.join(', ')}</dd></div>{/if}
					</dl>
				{/if}
				{#if info?.streaming.length}
					<div class="where-row" id="title-where"><span class="lbl">Where to watch</span>{@render services()}</div>
				{/if}
			</div>
			{@render folds()}
		{:else}
			<!-- A1 · Beside: the story and seasons on the left, the details card beside them (it stays in view while the seasons scroll). -->
			<div class="tp-body look-a" class:solo={!hasCard}>
				<div class="tp-main">
					{@render story()}
					{@render folds()}
				</div>
				{#if hasCard}<aside class="tp-side">{@render card()}</aside>{/if}
			</div>
		{/if}
		{@render castRow()}
		<TitleRows kind={d.kind} tmdb={d.tmdb} bind:info band={false} />

		<Sheet bind:open={allCast} title="Cast and crew" size="lg">
			<ul class="all-grid people">
				{#each d.cast as p, i (p.name + i)}
					<li><PersonCircle name={p.name} role={p.role} image={p.image} href={personHref(p.id)} /></li>
				{/each}
			</ul>
		</Sheet>

	</article>
{/if}
</PageLoader>
{/if}

<TitleLookPicker />

{#if manage}
	<AddMovieDialog bind:this={addMovie} {folders} {rules} {gateDefault} onadded={(_m, t) => added(t)} />
	<AddShowDialog bind:this={addShow} {folders} {rules} defaults={showDefaults} onadded={(_s, t) => added(t)} />
{/if}
{#if mayAsk}
	<AskMovieDialog bind:this={askMovieDialog} onasked={(_m, r) => movieAsked(r)} onrefused={(_m, body) => typeof body.title_id === 'number' && void goto(to(`/library/${body.title_id}`))} />
	<AskShowDialog bind:this={askShow} onasked={showAsked} onrefused={(_s, body) => typeof body.title_id === 'number' && void goto(to(`/library/${body.title_id}`))} />
{/if}

<style>
	.all-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(132px, 1fr)); gap: 18px 14px; margin: 0; padding: 0; list-style: none; }
	.all-grid.people { grid-template-columns: repeat(auto-fill, minmax(104px, 1fr)); }
	/* Layout A's details. Linked names read as text until hovered. */
	.tp :global(:is(.fact-list, .inline-facts) a) { color: inherit; text-decoration: none; }
	.tp :global(:is(.fact-list, .inline-facts) a:hover) { color: var(--text); text-decoration: underline; text-underline-offset: 3px; }
	.lbl { color: var(--text-muted); }
	.lead { max-width: 72ch; margin: 0; font: 400 16px/1.55 var(--font-ui); color: var(--text); }
	.svc { display: flex; flex-wrap: wrap; gap: 8px; margin: 0; padding: 0; list-style: none; }
	.svc li { display: inline-flex; align-items: center; gap: 8px; min-height: 36px; padding: 4px 12px 4px 4px; border-radius: 999px; background: var(--surface-2); font: 500 14px/1 var(--font-ui); }
	.svc img { width: 28px; height: 28px; border-radius: 7px; object-fit: cover; }
	/* A1 and A2: the details card. */
	.look-a .tp-main { gap: 22px; }
	.look-a.solo { grid-template-columns: minmax(0, 1fr); }
	.facts { display: flex; flex-direction: column; gap: 16px; padding: 16px 18px; border-radius: var(--r-card); background: var(--surface-1); border: 1px solid var(--line); position: sticky; top: calc(var(--topbar, 56px) + 16px); }
	.stacked .facts { position: static; }
	.fact-list { display: grid; grid-template-columns: auto minmax(0, 1fr); margin: 0; font: 400 14px/1.35 var(--font-ui); }
	.fact-list dt { padding: 10px 0; color: var(--text); font-weight: 600; }
	.fact-list dd { margin: 0; padding: 10px 0 10px 16px; color: var(--text-muted); text-align: right; overflow-wrap: anywhere; }
	.fact-list dt:not(:first-of-type), .fact-list dt:not(:first-of-type) + dd { border-top: 1px solid var(--line); }
	.fact-where .tp-h3 { font-size: 14px; }
	/* A3: the details as one row of small columns. */
	.strip { display: flex; flex-direction: column; gap: 20px; }
	.inline-facts { display: flex; flex-wrap: wrap; gap: 14px 40px; margin: 0; padding: 16px 0; border-block: 1px solid var(--line); }
	.inline-facts div { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
	.inline-facts dt { color: var(--text-muted); font: 600 11.5px/1.2 var(--font-ui); letter-spacing: .06em; text-transform: uppercase; }
	.inline-facts dd { margin: 0; color: var(--text); font: 500 15px/1.35 var(--font-ui); }
	.where-row { display: flex; flex-wrap: wrap; align-items: center; gap: 10px 14px; font: 500 14px/1 var(--font-ui); }
	@media (max-width: 767px) { .facts { position: static; } .inline-facts { gap: 12px 24px; } }
</style>
