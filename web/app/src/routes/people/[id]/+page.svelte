<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, message } from '#lib/api.ts';
	import { session } from '#lib/session.svelte.ts';
	import Poster from '#lib/Poster.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import { SkGrid, SkRail, SkRowTitle } from '#lib/ui/sk/index.ts';
	import Shelf from '#lib/ui/Shelf.svelte';
	import WideCard from '#lib/ui/WideCard.svelte';
	import BrowseTile from '#lib/ui/BrowseTile.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import type { CardOwned } from '#lib/ui/CardBadges.svelte';
	import Icon from '#lib/Icon.svelte';
	import { to } from '#lib/paths.ts';
	import { plainReason } from '#lib/plain.ts';
	import { detailPath } from '#lib/title-details.ts';
	import { followLabel, movieFollowHint, setFollow } from '#lib/follow.ts';
	import { byDecade, personChips, runReviewed, type FilmSort, type FilmographyPage, type PersonMovie, type PersonRole } from '#lib/people.ts';
	import type { AskLimit } from '#lib/types.ts';
	import { licenseURL, loadWikiFacts, wikipediaURL, type WikiFacts } from '#lib/wiki-facts.ts';
	import { cameFrom, fromIcon } from '#lib/came-from.svelte.ts';
	import { peopleSort, takePrefetched } from '#lib/person-prefetch.ts';

	const id = page.params.id ?? '';
	const SORT_KEY = 'holyarrs.people.sort';
	const savedSort = peopleSort;
	let role = $state<PersonRole>('all');
	let sort = $state<FilmSort>(savedSort());
	let data = $state<FilmographyPage | null>(null);
	// Every credit for the role, in the server's sort order (fetched page by page).
	let movies = $state<PersonMovie[]>([]);
	let limit = $state<AskLimit | null>(null);
	let busy = $state(false);
	let error = $state('');
	let asking = $state('');
	let done = $state<Record<string, string>>({});
	let bioOpen = $state(false);
	// Awards, death date and the Wikipedia bio (Wikidata via the metadata server).
	let wiki = $state<WikiFacts>({ badges: [], awards: [], bio: null });
	const bio = $derived(wiki.bio?.extract || data?.person.biography || '');
	// Follow state of the Coming soon movies not in the Library, by TMDB id.
	let follows = $state<Record<string, boolean>>({});
	let followBusy = $state('');
	let loadRevision = 0;
	const action = $derived(session.can('manage_library') ? 'add' : session.can('ask') ? 'ask' : null);

	async function load() {
		const revision = ++loadRevision;
		busy = true;
		error = '';
		try {
			const base = `/people/${encodeURIComponent(id)}?role=${role}&sort=${sort}&page_size=100`;
			// Opened from a face the pointer rested on: the first page is usually already here.
			const first = await ((role === 'all' && takePrefetched(`${base}&page=1`)) || api.get<FilmographyPage>(`${base}&page=1`));
			let all = first.movies;
			for (let p = 2; p <= first.total_pages; p++) all = all.concat((await api.get<FilmographyPage>(`${base}&page=${p}`)).movies);
			if (revision !== loadRevision) return;
			// The personal rows stay as the first answer drew them (all roles).
			data = data && role !== 'all' ? { ...first, library: data.library, asked: data.asked, known_for: data.known_for, coming_soon: data.coming_soon, often_with: data.often_with, stats: data.stats } : first;
			movies = all;
			if (role === 'all') void loadFollows(first.coming_soon ?? []);
		} catch (err) {
			if (revision === loadRevision) error = message(err);
		} finally {
			if (revision === loadRevision) busy = false;
		}
	}

	async function loadFollows(list: PersonMovie[]) {
		if (session.s.state !== 'signed_in') return;
		const want = list.filter((m) => !m.title_id && !(m.ids.tmdb in follows));
		const got = await Promise.all(want.map((m) => api.get<{ following: boolean }>(`/metadata/movie/${encodeURIComponent(m.ids.tmdb)}/follow`).then((r) => r.following).catch(() => false)));
		follows = { ...follows, ...Object.fromEntries(want.map((m, i) => [m.ids.tmdb, got[i]])) };
	}

	async function flipFollow(m: PersonMovie) {
		if (followBusy) return;
		followBusy = m.ids.tmdb;
		try {
			const on = await setFollow(api, { tmdb: m.ids.tmdb, kind: 'movie' }, !follows[m.ids.tmdb]);
			follows = { ...follows, [m.ids.tmdb]: on };
		} catch (err) {
			error = message(err);
		} finally {
			followBusy = '';
		}
	}

	function changeRole(next: PersonRole) {
		role = next;
		void load();
	}
	function changeSort(next: FilmSort) {
		sort = next;
		try {
			localStorage.setItem(SORT_KEY, next);
		} catch {
			/* private window: the choice lasts this visit */
		}
		void load();
	}

	async function quick(m: PersonMovie) {
		if (!action || asking) return;
		asking = m.ids.tmdb;
		const [r] = await runReviewed((path, body) => api.post(path, body), action, [m.ids.tmdb]);
		asking = '';
		done = { ...done, [m.ids.tmdb]: r?.ok ? (action === 'add' ? 'Added' : 'Asked') : (r?.error ?? 'Not sent') };
		if (action === 'ask') void api.get<AskLimit>('/asks/limit').then((x) => (limit = x)).catch(() => {});
	}

	const inLibrary: CardOwned = { state: 'available', label: 'In your Library' };
	function owned(m: PersonMovie): CardOwned | null {
		if (m.title_id) return inLibrary;
		if (done[m.ids.tmdb] === 'Asked' || done[m.ids.tmdb] === 'Added') return { state: 'requested', label: 'Requested' };
		if (!m.ask || m.ask.state === 'turned_down') return null;
		return { state: 'requested', label: m.ask.state === 'approved' ? 'Approved, waiting for a copy' : 'Requested' };
	}
	const href = (m: PersonMovie) => to(m.title_id ? `/library/${m.title_id}` : detailPath('movie', m.ids.tmdb));
	const roleLine = (m: PersonMovie) => {
		const who = m.characters?.length ? m.characters.join(' / ') : m.roles?.includes('Director') ? 'Director' : '';
		return [who, m.year ? String(m.year) : ''].filter(Boolean).join(' · ');
	};

	const hero = $derived(data?.known_for?.find((m) => m.backdrop)?.backdrop ?? null);
	let heroIn = $state(false);
	$effect(() => {
		void hero;
		heroIn = false;
	});
	const chips = $derived(personChips(data?.stats, data?.library?.length ?? 0, data?.asked?.length ?? 0));
	const facts = $derived.by(() => {
		const p = data?.person;
		if (!p) return [];
		const known = data?.known_for?.[0]?.name;
		const born = p.birthday ? new Date(`${p.birthday}T00:00:00Z`).toLocaleDateString(undefined, { year: 'numeric', month: 'long', day: 'numeric', timeZone: 'UTC' }) : '';
		const died = wiki.died && wiki.died.length === 10 ? new Date(`${wiki.died}T00:00:00Z`).toLocaleDateString(undefined, { year: 'numeric', month: 'long', day: 'numeric', timeZone: 'UTC' }) : wiki.died ?? '';
		return [known ? `Known for ${known}` : p.department, born ? `Born ${born}` : '', p.birthplace ? `From ${p.birthplace}` : wiki.birth_place ? `From ${wiki.birth_place}` : '', died ? `Died ${died}` : ''].filter(Boolean) as string[];
	});
	const decades = $derived.by(() => {
		const g = byDecade(movies);
		if (sort !== 'popular') return g;
		return g.sort((a, b) => (a.label === 'Coming soon' ? 1 : b.label === 'Coming soon' ? -1 : b.label.localeCompare(a.label)));
	});

	onMount(() => {
		void load();
		void loadWikiFacts(api.get, 'person', id).then((f) => (wiki = f));
		if (session.can('ask') && !session.can('manage_library')) void api.get<AskLimit>('/asks/limit').then((r) => (limit = r)).catch(() => {});
	});

	// Back to the page this person was opened from (a title's cast, Search…), with its scroll.
	const from = $derived.by(() => (void page.url.pathname, cameFrom()));
	function stepBack(e: MouseEvent) {
		if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
		e.preventDefault();
		history.back();
	}
</script>

<svelte:head><title>{data?.person.name ?? 'Person'} · HolyArrs</title>{#if hero}<link rel="preload" as="image" href={hero} />{/if}</svelte:head>

{#snippet filmCard(m: PersonMovie)}
	<WideCard cover title={m.name} href={href(m)} image={m.backdrop || m.poster} kind="movie" subtitle={roleLine(m) || undefined} owned={owned(m)} preview={{ kind: 'movie', tmdb: m.ids.tmdb, year: m.year || undefined, titleId: m.title_id }}>
		{#snippet growAction()}
			{#if action && !m.title_id && !m.ask && !m.never_add && !done[m.ids.tmdb]}
				<IconButton icon="plus" label={action === 'add' ? `Add ${m.name}` : `Ask for ${m.name}`} variant="glass" size="lg" aria-busy={asking === m.ids.tmdb || undefined} onclick={() => quick(m)} />
			{:else}
				<IconButton icon="play" label="Open {m.name}" variant="glass" size="lg" href={href(m)} />
			{/if}
			{#if !m.title_id && m.ids.tmdb in follows}
				{@const on = follows[m.ids.tmdb]}
				<IconButton icon="bell" label="{followLabel(on)} {m.name}" title={movieFollowHint(m.name, on)} variant="glass" size="lg" aria-pressed={on} aria-busy={followBusy === m.ids.tmdb || undefined} onclick={() => flipFollow(m)} />
			{/if}
		{/snippet}
	</WideCard>
{/snippet}

{#snippet row(title: string, list: PersonMovie[] | undefined, anchor: string)}
	{#if list?.length}
		<Shelf {title} {anchor} item="var(--title-item)" items={list} key={(m) => m.ids.tmdb} placeholder="wide" lines={1}>
			{#snippet card(m: PersonMovie)}{@render filmCard(m)}{/snippet}
		</Shelf>
	{/if}
{/snippet}

<div class="person">
{#if error && !data}<LoadError hasData={false} message={plainReason(error).text} onretry={load} {busy} />
{:else if error}<p class="notice bad" role="alert"><Icon name="alert" size={17} /><span>{plainReason(error).text}</span></p>{/if}
{#if busy && !data}
	<!-- Person-shaped placeholder: the same hero (portrait, name, facts, chips, bio), then Known for and the filmography. -->
	<div class="sk-page" aria-busy="true" aria-label="Loading this person" data-testid="person-skeleton">
		<div class="hero" data-theme="dark">
			<div class="hero-in">
				<span class="sk portrait-sk"></span>
				<div class="who">
					<span class="sk sk-line" style="height: clamp(32px, 5vw, 56px); width: min(420px, 80%); border-radius: 10px"></span>
					<span class="sk sk-line" style="width: 220px"></span>
					<span class="chips-sk"><span class="sk"></span><span class="sk"></span><span class="sk"></span></span>
					<span class="sk sk-line"></span><span class="sk sk-line"></span><span class="sk sk-line" style="width: 60%"></span>
				</div>
			</div>
		</div>
		<SkRail kind="wide" item="var(--title-item)" count={6} titleWidth="130px" />
		<div class="block"><SkRowTitle width="150px" /><SkGrid kind="wide" min="var(--title-item)" count={10} /></div>
	</div>
{/if}
{#if data}
	<header class="hero" data-hero>
		{#if hero}<img class="art" class:in={heroIn} src={hero} alt="" decoding="async" fetchpriority="high" onload={() => (heroIn = true)} onerror={() => (heroIn = false)} />{/if}
		<div class="shade" aria-hidden="true"></div>
		{#if from?.label}<nav class="crumbs" aria-label="Breadcrumb"><a class="chip" href={from.path} onclick={stepBack} data-testid="person-back"><Icon name={fromIcon(from.path) ?? 'chevronLeft'} size={14} />{from.label}</a></nav>{/if}
		<div class="hero-in">
			<div class="portrait"><Poster src={data.person.profile} name={data.person.name} /></div>
			<div class="who">
				<h1>{data.person.name}</h1>
				{#if facts.length}<p class="facts">{facts.join(' · ')}</p>{/if}
				{#if chips.length || wiki.badges.length}<ul class="chips" aria-label="At a glance">{#each wiki.badges.slice(0, 3) as c (c)}<li class="award" data-testid="award-badge">{c}</li>{/each}{#each chips as c (c)}<li>{c}</li>{/each}</ul>{/if}
				{#if bio}
					<p class="bio" class:open={bioOpen}>{bio}</p>
					<button class="more" type="button" onclick={() => (bioOpen = !bioOpen)}>{bioOpen ? 'Less' : 'More'}</button>
					{#if wiki.bio}<p class="credit" data-testid="wiki-credit">From <a href={wikipediaURL(wiki.bio.url) || 'https://en.wikipedia.org/'} target="_blank" rel="noopener noreferrer">Wikipedia</a>, <a href={licenseURL(wiki.bio.license_url)} target="_blank" rel="noopener noreferrer license">CC BY-SA</a></p>{/if}
				{/if}
			</div>
		</div>
	</header>

	{#if limit && action === 'ask' && !limit.no_limit && !limit.movies.no_limit}
		<p class="note">Movie asks this rolling week: {limit.movies.used} used, {limit.movies.left} left.</p>
	{/if}

	{@render row('Coming soon', data.coming_soon, 'person-soon')}
	{@render row('Known for', data.known_for, 'person-known')}
	{@render row('In your Library', data.library, 'person-library')}
	{@render row('You asked for', data.asked, 'person-asked')}

	{#if data.often_with?.length}
		<section class="block" aria-labelledby="with-title">
			<h2 id="with-title">Often works with</h2>
			<ul class="tiles">
				{#each data.often_with as p (p.id)}
					<li><BrowseTile name={p.name} href={to(`/people/${p.id.replace(/^tmdb:/, '')}`)} image={p.profile ?? ''} hue="#555b66" plain /></li>
				{/each}
			</ul>
		</section>
	{/if}

	<section class="block" aria-labelledby="films-title">
		<div class="toolbar">
			<h2 id="films-title">Filmography</h2>
			<div class="controls">
				<SegmentedControl label="Credit role" value={role} options={[{ value: 'all', label: 'All' }, { value: 'actor', label: 'Actor' }, { value: 'director', label: 'Director' }]} onchange={(v) => changeRole(v as PersonRole)} />
				<SegmentedControl label="Sort" value={sort} options={[{ value: 'oldest', label: 'Oldest first' }, { value: 'newest', label: 'Newest first' }, { value: 'popular', label: 'Popular' }]} onchange={(v) => changeSort(v as FilmSort)} />
			</div>
		</div>
		{#each decades as g (g.label)}
			<h3 class="decade">{g.label}</h3>
			<ul class="grid" aria-busy={busy || undefined}>
				{#each g.movies as m (m.ids.tmdb)}<li>{@render filmCard(m)}</li>{/each}
			</ul>
		{/each}
		{#if !movies.length && !busy}<EmptyState compact icon="film" title="No movies match this role." />{/if}
	</section>

	<p class="attribution">{#if !data.complete}Credits as the provider lists them; it may not be complete. {/if}{data.attribution}</p>
{/if}
</div>

<style>
	.person { --title-item: max(200px, calc((100vw - 96px) / 5.16)); display: flex; flex-direction: column; gap: 32px; padding-bottom: 48px; min-width: 0; }
	/* The hero stays dark in both themes, like the title pages. */
	.hero { position: relative; isolation: isolate; min-height: clamp(380px, 38vw, 560px); display: flex; align-items: flex-end; overflow: hidden; margin: calc(var(--s-6) * -1) calc(var(--gutter) * -1) 0; background: var(--on-art-ink); color: var(--on-art); }
	.art { position: absolute; inset: 0; z-index: -2; width: 100%; height: 100%; object-fit: cover; object-position: center 25%; }
	/* The hero image fades in over 400 ms once loaded (Silhouette image rules). */
	.art { opacity: 0; transition: opacity 400ms ease-out; }
	.art.in { opacity: 1; }
	.sk-page { display: flex; flex-direction: column; gap: 32px; min-width: 0; }
	.portrait-sk { aspect-ratio: var(--poster-ratio); width: 100%; }
	.chips-sk { display: flex; gap: 8px; }
	.chips-sk .sk { width: 84px; height: 28px; border-radius: 999px; }
	.who .sk-line { width: 100%; max-width: 72ch; }
	.shade { position: absolute; inset: 0; z-index: -1; background: linear-gradient(to top, var(--on-art-scrim) 0%, var(--on-art-scrim-mid) 38%, var(--on-art-scrim-none) 72%), linear-gradient(to right, var(--on-art-scrim-mid) 0%, var(--on-art-scrim-none) 60%); }
	.hero-in { width: 100%; display: grid; grid-template-columns: clamp(120px, 14vw, 200px) minmax(0, 720px); gap: clamp(18px, 3vw, 36px); align-items: end; padding: clamp(20px, 4vw, 48px); }
	.portrait { border-radius: var(--r-card); overflow: hidden; box-shadow: 0 18px 48px rgb(0 0 0 / .5); }
	.crumbs { position: absolute; top: clamp(14px, 2vw, 22px); left: clamp(20px, 4vw, 48px); z-index: 1; }
	.chip { display: inline-flex; align-items: center; gap: 6px; height: 30px; padding: 0 12px 0 10px; border-radius: 999px; background: rgb(255 255 255 / 14%); border: 1px solid rgb(255 255 255 / 18%); backdrop-filter: blur(18px) saturate(160%); -webkit-backdrop-filter: blur(18px) saturate(160%); color: #fff; font: 600 13px/1.2 var(--font-ui); transition: background var(--dur-fast) var(--ease-out); }
	.chip:hover { background: rgb(255 255 255 / 24%); }
	.chip:focus-visible { outline: none; box-shadow: 0 0 0 2px #fff; }
	.who { min-width: 0; display: flex; flex-direction: column; gap: 10px; }
	h1 { margin: 0; font: var(--t-large-title); font-family: var(--font-display); font-size: clamp(32px, 5vw, 60px); letter-spacing: var(--t-large-title-track); text-wrap: balance; }
	.facts { margin: 0; color: var(--on-art-muted); font: var(--t-subhead); }
	.chips { display: flex; flex-wrap: wrap; gap: 8px; margin: 2px 0 0; padding: 0; list-style: none; }
	.chips li { padding: 5px 12px; border-radius: 999px; background: rgb(255 255 255 / .14); backdrop-filter: blur(12px); font: var(--t-footnote); font-weight: 600; font-variant-numeric: tabular-nums; }
	.bio { margin: 0; max-width: 72ch; font: var(--t-callout); line-height: 1.55; color: var(--on-art); display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
	.bio.open { display: block; }
	.credit { margin: 4px 0 0; font: var(--t-caption, 12px/1.4 var(--font-ui)); color: var(--on-art); opacity: .7; }
	.credit a { color: inherit; text-decoration: underline; }
	.chips li.award { color: #f5c518; }
	.more { align-self: flex-start; padding: 0; border: 0; background: none; color: var(--on-art); font: var(--t-subhead); font-weight: 600; cursor: pointer; text-decoration: underline; }
	.block { display: flex; flex-direction: column; gap: 14px; min-width: 0; }
	.block h2 { margin: 0; font: var(--t-title2); }
	.toolbar { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px; }
	.controls { display: flex; flex-wrap: wrap; gap: 10px; }
	.decade { margin: 8px 0 0; font: var(--t-title3); color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(var(--title-item), 1fr)); gap: 16px 5px; margin: 0; padding: 0; list-style: none; }
	.tiles { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: 16px; margin: 0; padding: 0; list-style: none; }
	.note { margin: 0; color: var(--text-muted); font: var(--t-subhead); font-weight: 400; font-variant-numeric: tabular-nums; }
	.notice { display: flex; align-items: center; gap: 8px; margin: 0; padding: 10px 12px; border-radius: var(--r-btn); background: var(--surface-2); font: var(--t-callout); }
	.notice.bad { color: var(--fail); }
	.attribution { margin: 0; color: var(--text-muted); font: var(--t-footnote); }
	@media (max-width: 767px) {
		.person { --title-item: calc((100cqi - var(--shelf-gap, 12px)) / 1.6 / .78); gap: 26px; }
		.hero-in { grid-template-columns: 96px minmax(0, 1fr); }
		.grid { grid-template-columns: repeat(auto-fill, minmax(min(100%, 260px), 1fr)); }
		.tiles { grid-template-columns: repeat(3, minmax(0, 1fr)); }
	}
</style>
