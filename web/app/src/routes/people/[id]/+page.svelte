<script lang="ts">
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, message } from '#lib/api.ts';
	import { session } from '#lib/session.svelte.ts';
	import Poster from '#lib/Poster.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import WideCard from '#lib/ui/WideCard.svelte';
	import BrowseTile from '#lib/ui/BrowseTile.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import type { CardOwned } from '#lib/ui/CardBadges.svelte';
	import Icon from '#lib/Icon.svelte';
	import { to } from '#lib/paths.ts';
	import { plainReason } from '#lib/plain.ts';
	import { detailPath } from '#lib/title-details.ts';
	import { followsByTmdb } from '#lib/explore.ts';
	import { byDecade, personChips, runReviewed, type FilmSort, type FilmographyPage, type PersonMovie, type PersonRole } from '#lib/people.ts';
	import type { AskLimit } from '#lib/types.ts';
	import { loadWikiFacts, type WikiFacts } from '#lib/wiki-facts.ts';
	import { setTopCrumbs } from '#lib/top-crumbs.svelte.ts';
	import { peopleSort, takePrefetched } from '#lib/person-prefetch.ts';
	import VariantBar from '#lib/ui/VariantBar.svelte';
	import PersonCircle from '#lib/ui/PersonCircle.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import { paintWindow, sampleCover, tintOf, unpaintWindow } from '#lib/music/album-tint.ts';

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
	// The hero waits for the wiki facts (awards, bio) a short moment, so its words do not move when they land.
	let wikiDone = $state(false);
	let wikiWaited = $state(false);
	const WIKI_WAIT_MS = 600;
	const bio = $derived(wiki.bio?.extract || data?.person.biography || '');
	// Follow state of the Coming soon movies not in the Library, by TMDB id.
	let loadRevision = 0;
	const action = $derived(session.can('manage_library') ? 'add' : session.can('ask') ? 'ask' : null);

	/** Coming soon movies: their hover card's bell follows them (GrowCard's own bell, after the state). */
	const coming = $derived(new Set((data?.coming_soon ?? []).map((m) => m.ids.tmdb)));

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
		} catch (err) {
			if (revision === loadRevision) error = message(err);
		} finally {
			if (revision === loadRevision) busy = false;
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

	// No TMDB photo: ask again with the name, so the server can look for a backup photo by name too.
	$effect(() => {
		const n = data?.person.name;
		if (!n || data?.person.profile || wiki.image) return;
		void loadWikiFacts(api.get, 'person', id, n).then((f) => {
			if (f.image) wiki = { ...wiki, image: f.image };
		});
	});
	onMount(() => {
		void load();
		void loadWikiFacts(api.get, 'person', id).then((f) => (wiki = f)).finally(() => (wikiDone = true));
		const waited = setTimeout(() => (wikiWaited = true), WIKI_WAIT_MS);
		if (session.can('ask') && !session.can('manage_library')) void api.get<AskLimit>('/asks/limit').then((r) => (limit = r)).catch(() => {});
		return () => clearTimeout(waited);
	});

	// Three layouts Luis is comparing (?look=1|2|3, 0 = today's page): 1 a movie-like
	// billboard, 2 the artist-like banner with a tinted page, 3 a credits list.
	const VIEWS = [{ value: 0, label: 'Now' }, { value: 1, label: 'P1 · Billboard' }, { value: 2, label: 'P2 · Round' }, { value: 4, label: 'P2 · Square' }, { value: 5, label: 'P2 · Portrait' }, { value: 3, label: 'P3 · Credits' }];
	let view = $state(0);
	// P2 comes in three photo shapes (round, square, 2:3 portrait); 4 and 5 are P2 too.
	const mix = $derived(view === 2 || view === 4 || view === 5);
	const shape = $derived(view === 4 ? 'square' : view === 5 ? 'portrait' : 'round');
	const photo = $derived(data?.person.profile || wiki.image || '');
	const yearsLine = $derived.by(() => {
		const st = data?.stats;
		return st?.first_year ? `${st.first_year}–${st.active ? 'now' : st.last_year}` : '';
	});
	// One line under the name: what they do, then born and from.
	const lead = $derived([data?.person.department, ...facts.filter((f) => !f.startsWith('Known for'))].filter(Boolean).join(' · '));
	let aboutOpen = $state(false);
	let allDecade = $state<{ label: string; movies: PersonMovie[] } | null>(null);

	// View 2: the banner zooms out and darkens as it leaves; the page takes its colour.
	let heroBox: HTMLElement | undefined = $state();
	let gone = $state(0);
	function measure() {
		if (!heroBox) return;
		const r = heroBox.getBoundingClientRect();
		gone = Math.min(1, Math.max(0, -r.top / Math.max(1, r.height)));
	}
	$effect(() => {
		if (!mix) return;
		measure();
		addEventListener('scroll', measure, { passive: true, capture: true });
		return () => removeEventListener('scroll', measure, { capture: true });
	});
	let sampled = $state<string | null>(null);
	$effect(() => {
		const src = photo || hero;
		if (!mix || !src || sampled) return;
		const img = new Image();
		img.onload = () => (sampled = sampleCover(img));
		img.src = src;
	});
	const pageTint = $derived(mix && sampled ? tintOf(sampled) : null);
	$effect(() => {
		if (pageTint) {
			paintWindow(pageTint);
			return () => unpaintWindow();
		}
	});

	// Where this person was opened from is in the top bar (layout), like every page.
	setTopCrumbs(null);
</script>

<svelte:head><title>{data?.person.name ?? 'Person'} · HolyArrs</title>{#if hero}<link rel="preload" as="image" href={hero} />{/if}</svelte:head>

{#snippet filmCard(m: PersonMovie)}
	<WideCard cover title={m.name} href={href(m)} image={m.backdrop || m.poster} kind="movie" subtitle={roleLine(m) || undefined} owned={owned(m)} preview={{ kind: 'movie', tmdb: m.ids.tmdb, year: m.year || undefined, titleId: m.title_id }}
		followTmdb={!m.title_id && (coming.has(m.ids.tmdb) || followsByTmdb({ kind: 'movie', titleId: m.title_id, year: m.year }))}>
		{#snippet growAction()}
			{#if action && !m.title_id && !m.ask && !m.never_add && !done[m.ids.tmdb]}
				<IconButton icon="plus" label={action === 'add' ? `Add ${m.name}` : `Ask for ${m.name}`} variant="glass" size="lg" aria-busy={asking === m.ids.tmdb || undefined} onclick={() => quick(m)} />
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
<PageLoader loading={(!data || !(wikiDone || wikiWaited)) && !error} label="Loading this person" onretry={load}>
{#if data && (wikiDone || wikiWaited)}
	{#if view === 0}
		<header class="hero" data-hero>
			{#if hero}<img class="art" class:in={heroIn} src={hero} alt="" decoding="async" fetchpriority="high" onload={() => (heroIn = true)} onerror={() => (heroIn = false)} />{/if}
			<div class="shade" aria-hidden="true"></div>
			<div class="hero-in">
				<div class="portrait"><Poster src={data.person.profile || wiki.image} name={data.person.name} /></div>
				<div class="who">
					<h1>{data.person.name}</h1>
					{#if facts.length}<p class="facts">{facts.join(' · ')}</p>{/if}
					{#if chips.length || wiki.badges.length}<ul class="chips" aria-label="At a glance">{#each wiki.badges.slice(0, 3) as c (c)}<li class="award" data-testid="award-badge">{c}</li>{/each}{#each chips as c (c)}<li>{c}</li>{/each}</ul>{/if}
					{#if bio}
						<p class="bio" class:open={bioOpen}>{bio}</p>
						<button class="more" type="button" onclick={() => (bioOpen = !bioOpen)}>{bioOpen ? 'Less' : 'More'}</button>
					{/if}
				</div>
			</div>
		</header>
	{:else if mix}
		<header class="mix" bind:this={heroBox} style:--gone={gone.toFixed(3)} data-hero data-testid="person-banner">
			<div class="stage" aria-hidden="true">{#if hero}<img class="art" class:in={heroIn} src={hero} alt="" decoding="async" fetchpriority="high" onload={() => (heroIn = true)} />{/if}</div>
			<div class="m-shade" aria-hidden="true"></div>
			<div class="dim" aria-hidden="true"></div>
		</header>
		<div class="cline">
			<span class="round {shape}">{#if photo}<img src={photo} alt="" />{:else}<span>{data.person.name.split(' ').map((w) => w[0]).slice(0, 2).join('')}</span>{/if}</span>
			<div class="c-words">
				<h1>{data.person.name}</h1>
				{#if lead}<p class="c-line">{lead}</p>{/if}
			</div>
		</div>
		<section class="stats" aria-label="At a glance" data-testid="person-stats">
			<p class="st1">
				{#if data.stats?.films}<span><b>{data.stats.films}</b> {data.stats.films === 1 ? 'film' : 'films'}</span>{/if}
				{#if data.stats?.shows}<span><b>{data.stats.shows}</b> {data.stats.shows === 1 ? 'show' : 'shows'}</span>{/if}
				{#if yearsLine}<span><b>{yearsLine}</b></span>{/if}
			</p>
			{#if data.library?.length || data.asked?.length || wiki.badges.length}
				<p class="st2">
					{#if data.library?.length}<span class="own"><Icon name="library" size={14} />{data.library.length} in your Library</span>{/if}
					{#if data.asked?.length}<span>{data.asked.length} asked for</span>{/if}
					{#each wiki.badges.slice(0, 2) as c (c)}<span class="award">{c}</span>{/each}
				</p>
			{/if}
		</section>
	{:else}
		<header class="bill" class:short={view === 3} data-hero data-testid="person-billboard">
			{#if hero}<img class="art" class:in={heroIn} src={hero} alt="" decoding="async" fetchpriority="high" onload={() => (heroIn = true)} />{/if}
			<div class="b-shade" aria-hidden="true"></div>
			<div class="b-in">
				{#if photo}<span class="face"><img src={photo} alt="" /></span>{/if}
				<h1>{data.person.name}</h1>
				{#if lead}<p class="b-line">{lead}</p>{/if}
				{#if view === 1}
					{#if chips.length || wiki.badges.length}<p class="b-facts">{#each wiki.badges.slice(0, 3) as c (c)}<span class="award">{c}</span>{/each}{#each chips as c (c)}<span>{c}</span>{/each}</p>{/if}
					{#if bio}
						<p class="bio" class:open={bioOpen}>{bio}</p>
						<button class="more b-more" type="button" onclick={() => (bioOpen = !bioOpen)}>{bioOpen ? 'LESS' : 'MORE'}</button>
					{/if}
				{/if}
			</div>
		</header>
	{/if}


	{#if limit && action === 'ask' && !limit.no_limit && !limit.movies.no_limit}
		<p class="note">Movie asks this rolling week: {limit.movies.used} used, {limit.movies.left} left.</p>
	{/if}

	{#if view === 3}
		{@render row('Known for', data.known_for, 'person-known')}
		{@render row('Coming soon', data.coming_soon, 'person-soon')}
	{:else}
		{@render row('Coming soon', data.coming_soon, 'person-soon')}
		{@render row('Known for', data.known_for, 'person-known')}
	{/if}
	{@render row('In your Library', data.library, 'person-library')}
	{@render row('You asked for', data.asked, 'person-asked')}

	{#if view === 0}
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

	{:else if data.often_with?.length}
		<section class="block" aria-labelledby="with-title">
			<h2 id="with-title">Often works with</h2>
			<ul class="circles">{#each data.often_with as p (p.id)}<li><PersonCircle name={p.name} image={p.profile ?? ''} href={to(`/people/${p.id.replace(/^tmdb:/, '')}`)} size={112} /></li>{/each}</ul>
		</section>
	{/if}

	{#snippet controls()}
		<div class="controls">
			<SegmentedControl label="Credit role" value={role} options={[{ value: 'all', label: 'All' }, { value: 'actor', label: 'Actor' }, { value: 'director', label: 'Director' }]} onchange={(v) => changeRole(v as PersonRole)} />
			<SegmentedControl label="Sort" value={sort} options={[{ value: 'oldest', label: 'Oldest first' }, { value: 'newest', label: 'Newest first' }, { value: 'popular', label: 'Popular' }]} onchange={(v) => changeSort(v as FilmSort)} />
		</div>
	{/snippet}

	{#if mix}
		<section class="block" aria-labelledby="films-title">
			<div class="toolbar"><h2 id="films-title">Filmography</h2>{@render controls()}</div>
		</section>
		{#each decades as g (g.label)}
			<Shelf title={g.label} item="var(--title-item)" items={g.movies.slice(0, 12)} key={(m) => m.ids.tmdb} placeholder="wide" lines={1} onseeall={g.movies.length > 12 ? () => (allDecade = g) : undefined}>
				{#snippet card(m: PersonMovie)}{@render filmCard(m)}{/snippet}
			</Shelf>
		{/each}
		{#if !movies.length && !busy}<EmptyState compact icon="film" title="No movies match this role." />{/if}
	{:else if view === 3}
		<section class="block" aria-labelledby="films-title">
			<div class="toolbar"><h2 id="films-title">Filmography</h2>{@render controls()}</div>
			<ol class="credits" aria-busy={busy || undefined} data-testid="person-credits">
				{#each movies as m (m.ids.tmdb)}
					{@const o = owned(m)}
					<li>
						<span class="cy">{m.year || 'Soon'}</span>
						<a class="ct" href={href(m)}>{m.name}</a>
						<span class="cr">{m.characters?.length ? m.characters.join(' / ') : (m.roles ?? []).join(', ')}</span>
						<span class="cs">{#if o}<span class="state" class:have={o.state === 'available'}>{o.state === 'available' ? 'In Library' : 'Requested'}</span>{:else if action && !m.never_add && !done[m.ids.tmdb]}<IconButton icon="plus" label={action === 'add' ? `Add ${m.name}` : `Ask for ${m.name}`} size="sm" aria-busy={asking === m.ids.tmdb || undefined} onclick={() => quick(m)} />{/if}</span>
					</li>
				{/each}
			</ol>
			{#if !movies.length && !busy}<EmptyState compact icon="film" title="No movies match this role." />{/if}
		</section>
	{:else}
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

	{/if}

	{#if view >= 2 && (bio || facts.length)}
		<section class="block about" aria-labelledby="about-title">
			<h2 id="about-title">About</h2>
			{#if mix}
				<button type="button" class="acard" aria-haspopup="dialog" aria-label="About {data.person.name}: read more" onclick={() => (aboutOpen = true)} data-testid="person-about">
					{#if photo || hero}<img src={hero || photo} alt="" loading="lazy" decoding="async" />{/if}
					<span class="a-foot"><b>{data.person.name}</b>{#if bio}<span class="a-bio">{bio}</span>{/if}</span>
				</button>
			{:else}
				<div class="info" data-testid="person-info">
					{#if bio}<p class="i-bio">{bio}</p>{/if}
					<dl>
						{#if data.known_for?.[0]}<div><dt>Known for</dt><dd>{data.known_for[0].name}</dd></div>{/if}
						{#each facts.filter((f) => !f.startsWith('Known for')) as f (f)}<div><dt>{f.split(' ')[0]}</dt><dd>{f.slice(f.indexOf(' ') + 1)}</dd></div>{/each}
						{#if yearsLine}<div><dt>Active</dt><dd>{yearsLine}</dd></div>{/if}
						{#if wiki.badges.length}<div><dt>Awards</dt><dd>{wiki.badges.slice(0, 3).join(' · ')}</dd></div>{/if}
					</dl>
				</div>
			{/if}
		</section>
		<Sheet bind:open={aboutOpen} title="About {data.person.name}" size="lg">
			{#if facts.length}<p class="s-facts">{facts.join(' · ')}</p>{/if}
			<p class="s-bio">{bio}</p>
		</Sheet>
	{/if}
	{#if allDecade}
		<Sheet open={true} onclose={() => (allDecade = null)} title={allDecade.label} size="lg">
			<ul class="grid">{#each allDecade.movies as m (m.ids.tmdb)}<li>{@render filmCard(m)}</li>{/each}</ul>
		</Sheet>
	{/if}

	{#if !data.complete}<p class="attribution">This list may not be complete.</p>{/if}
{/if}
</PageLoader>
<VariantBar options={VIEWS} bind:value={view} param="look" storageKey="holyarrs.people-look" label="People page layout" />
</div>

<style>
	.person { --title-item: max(200px, calc((100vw - 96px) / 5.16)); display: flex; flex-direction: column; gap: 32px; padding-bottom: 48px; min-width: 0; }
	/* The hero stays dark in both themes, like the title pages. */
	.hero { position: relative; isolation: isolate; min-height: clamp(380px, 38vw, 560px); display: flex; align-items: flex-end; overflow: hidden; margin: calc(var(--s-6) * -1) calc(var(--gutter) * -1) 0; background: var(--on-art-ink); color: var(--on-art); }
	.art { position: absolute; inset: 0; z-index: -2; width: 100%; height: 100%; object-fit: cover; object-position: center 25%; }
	/* The hero image fades in over 400 ms once loaded (Silhouette image rules). */
	.art { opacity: 0; transition: opacity var(--dur-slow) var(--ease-out); }
	.art.in { opacity: 1; }
	.shade { position: absolute; inset: 0; z-index: -1; background: linear-gradient(to top, var(--on-art-scrim) 0%, var(--on-art-scrim-mid) 38%, var(--on-art-scrim-none) 72%), linear-gradient(to right, var(--on-art-scrim-mid) 0%, var(--on-art-scrim-none) 60%); }
	.hero-in { width: 100%; display: grid; grid-template-columns: clamp(120px, 14vw, 200px) minmax(0, 720px); gap: clamp(18px, 3vw, 36px); align-items: end; padding: clamp(20px, 4vw, 48px); }
	.portrait { border-radius: var(--r-card); overflow: hidden; box-shadow: 0 18px 48px rgb(0 0 0 / .5); }
	.who { min-width: 0; display: flex; flex-direction: column; gap: 10px; }
	h1 { margin: 0; font: var(--t-large-title); font-family: var(--font-display); font-size: clamp(32px, 5vw, 60px); letter-spacing: var(--t-large-title-track); text-wrap: balance; }
	.facts { margin: 0; color: var(--on-art-muted); font: var(--t-subhead); }
	.chips { display: flex; flex-wrap: wrap; gap: 8px; margin: 2px 0 0; padding: 0; list-style: none; }
	.chips li { padding: 5px 12px; border-radius: 999px; background: rgb(255 255 255 / .14); backdrop-filter: blur(12px); font: var(--t-footnote); font-weight: 600; font-variant-numeric: tabular-nums; }
	.bio { margin: 0; max-width: 72ch; font: var(--t-callout); line-height: 1.55; color: var(--on-art); display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
	.bio.open { display: block; }
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
	.award { color: #f5c518; }
	/* P1/P3 · Billboard: the movie page hero, name as the title in the left 40 %. */
	.bill { position: relative; isolation: isolate; min-height: clamp(460px, 52vw, 720px); display: flex; align-items: flex-end; overflow: hidden; margin: calc(var(--s-6) * -1) calc(var(--gutter) * -1) 0; background: var(--on-art-ink); color: var(--on-art); }
	.bill.short { min-height: clamp(340px, 34vw, 480px); }
	.bill .art, .mix .art { position: absolute; inset: 0; z-index: -2; width: 100%; height: 100%; object-fit: cover; object-position: center 25%; }
	.b-shade { position: absolute; inset: 0; z-index: -1; background: linear-gradient(to top, var(--on-art-scrim) 0%, var(--on-art-scrim-mid) 30%, var(--on-art-scrim-none) 62%), linear-gradient(to right, var(--on-art-scrim) 0%, var(--on-art-scrim-mid) 30%, var(--on-art-scrim-none) 58%); }
	.b-in { display: flex; flex-direction: column; align-items: flex-start; gap: 10px; width: min(44%, 620px); padding: 0 var(--gutter) clamp(28px, 4vw, 56px); }
	.face { width: 64px; height: 64px; border-radius: 50%; overflow: hidden; box-shadow: 0 0 0 2px rgb(255 255 255 / .5), 0 8px 24px rgb(0 0 0 / .4); }
	.face img, .round img { width: 100%; height: 100%; object-fit: cover; }
	.bill h1 { font-size: clamp(36px, 5.4vw, 72px); line-height: 1; letter-spacing: -.03em; }
	.b-line { margin: 0; color: var(--on-art-muted); font: var(--t-subhead); font-weight: 600; }
	.b-facts { display: flex; flex-wrap: wrap; gap: 0; margin: 0; color: var(--on-art-muted); font: var(--t-footnote); font-weight: 600; font-variant-numeric: tabular-nums; }
	.b-facts span + span::before, .st1 span + span::before, .st2 span + span::before { content: '·'; margin: 0 10px; opacity: .6; }
	.b-more { text-decoration: none; font-family: var(--font-pixel, inherit); letter-spacing: .16em; font-size: 12px; padding: 4px 2px; }
	/* P2 · Mixture: the artist page's short banner fading into a tinted page. */
	.mix { position: relative; isolation: isolate; overflow: hidden; height: clamp(280px, 44vh, 520px); margin: calc(var(--s-6) * -1) calc(var(--gutter) * -1) 0; background: var(--tint-bg, #07080a); -webkit-mask-image: linear-gradient(to bottom, #000 55%, rgb(0 0 0 / .5) 80%, transparent 100%); mask-image: linear-gradient(to bottom, #000 55%, rgb(0 0 0 / .5) 80%, transparent 100%); }
	.stage { position: absolute; inset: 0; z-index: -2; scale: calc(1.1 - .1 * var(--gone, 0)); transform-origin: 50% 40%; }
	.mix .art { opacity: 0; transition: opacity var(--dur-slow) var(--ease-out); }
	.mix .art.in { opacity: .85; }
	.m-shade { position: absolute; inset: 0; z-index: -1; background: linear-gradient(to bottom, rgb(7 8 10 / .4), rgb(7 8 10 / 0) 30%); }
	.dim { position: absolute; inset: 0; z-index: -1; background: var(--tint-bg, #07080a); opacity: calc(var(--gone, 0) * .85); }
	@media (prefers-reduced-motion: reduce) { .stage { scale: 1; } }
	/* One centre line from the photo down to the figures (symmetry). */
	.cline { display: flex; flex-direction: column; align-items: center; gap: 14px; margin-top: calc(-32px - 96px); position: relative; z-index: 1; text-align: center; }
	.round { flex: none; display: grid; place-items: center; width: 168px; height: 168px; border-radius: 50%; overflow: hidden; background: var(--surface-2); box-shadow: 0 0 0 1px rgb(255 255 255 / .14), 0 18px 44px rgb(0 0 0 / .45); font: var(--t-title2); transition: border-radius var(--dur) var(--ease-out); }
	.round.square { border-radius: 22px; }
	.round.portrait { width: 152px; height: 228px; border-radius: 16px; }
	.c-words { display: flex; flex-direction: column; align-items: center; gap: 6px; min-width: 0; }
	.cline h1 { font-family: var(--font-display); font-weight: 800; font-size: clamp(34px, 4vw, 52px); line-height: 1; letter-spacing: -.03em; color: var(--text); text-wrap: balance; }
	.c-line { margin: 0; color: var(--text-muted); font: 600 15px/1.3 var(--font-ui); font-variant-numeric: tabular-nums; }
	.stats { display: flex; flex-direction: column; align-items: center; gap: 8px; width: min(100%, 720px); margin-inline: auto; padding: 24px 16px; border-radius: var(--r-card); background: var(--surface-1); text-align: center; font-variant-numeric: tabular-nums; }
	.stats p { margin: 0; display: flex; flex-wrap: wrap; justify-content: center; }
	.st1 { font: var(--t-title3); font-weight: 400; }
	.st1 b { font-weight: 700; }
	.st2 { color: var(--text-muted); font: var(--t-subhead); }
	.own { display: inline-flex; align-items: center; gap: 6px; }
	.acard { position: relative; display: block; width: 100%; height: 300px; padding: 0; border: 0; border-radius: var(--r-card); overflow: hidden; background: var(--surface-2); color: #fff; text-align: left; cursor: pointer; }
	.acard img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; transition: scale var(--dur-slow) var(--ease-out); }
	.acard::after { content: ''; position: absolute; inset: 0; background: linear-gradient(to top, rgb(0 0 0 / .78), rgb(0 0 0 / 0) 70%); }
	.acard:hover img { scale: 1.03; }
	.acard:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.a-foot { position: absolute; z-index: 1; left: 24px; right: 24px; bottom: 22px; display: flex; flex-direction: column; gap: 6px; }
	.a-foot b { font: var(--t-title2); }
	.a-bio { font: var(--t-callout); display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; max-width: 80ch; }
	.s-facts { margin: 0 0 12px; color: var(--text-muted); font: var(--t-subhead); }
	.s-bio { margin: 0; font: var(--t-body); line-height: 1.6; white-space: pre-line; }
	.circles { display: flex; gap: 20px; margin: 0; padding: 0 0 4px; list-style: none; overflow-x: auto; scrollbar-width: none; }
	.circles li { flex: none; }
	/* P3 · Credits: one dense list, then About with an Information column. */
	.credits { margin: 0; padding: 0; list-style: none; border-top: 1px solid var(--border); }
	.credits li { display: grid; grid-template-columns: 64px minmax(0, 2fr) minmax(0, 2fr) 120px; align-items: center; gap: 16px; min-height: 52px; border-bottom: 1px solid var(--border); font: var(--t-callout); }
	.cy { color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.ct { color: var(--text); font-weight: 600; text-decoration: none; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.ct:hover { text-decoration: underline; }
	.cr { color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.cs { display: flex; justify-content: flex-end; }
	.state { color: var(--text-muted); font: var(--t-footnote); font-weight: 600; }
	.state.have { color: var(--ok, #34c759); }
	.info { display: grid; grid-template-columns: minmax(0, 3fr) minmax(0, 2fr); gap: 32px; }
	.i-bio { margin: 0; font: var(--t-callout); line-height: 1.6; color: var(--text); }
	.info dl { margin: 0; display: flex; flex-direction: column; gap: 14px; }
	.info dt { color: var(--text-muted); font: var(--t-footnote); font-weight: 600; }
	.info dd { margin: 2px 0 0; font: var(--t-callout); }
	@media (max-width: 767px) {
		.bill { min-height: 440px; margin-top: calc(var(--s-4) * -1); }
		.bill.short { min-height: 340px; }
		.b-in { width: 100%; align-items: center; text-align: center; }
		.b-facts { justify-content: center; }
		.b-more { align-self: center; }
		.mix { height: 180px; margin-top: calc(var(--s-4) * -1); }
		.cline { gap: 12px; margin-top: calc(-26px - 64px); }
		.round { width: 120px; height: 120px; }
		.round.portrait { width: 112px; height: 168px; }
		.c-words { align-items: center; padding: 0; }
		.cline h1 { font-size: clamp(28px, 8.4vw, 36px); }
		.stats { padding: 20px 8px; }
		.st1 { font-size: 17px; }
		.acard { height: 320px; }
		.a-foot { left: 16px; right: 16px; bottom: 16px; }
		.credits li { grid-template-columns: 44px minmax(0, 1fr) auto; grid-template-rows: auto auto; row-gap: 0; padding: 8px 0; }
		.cy { grid-row: span 2; }
		.cr { grid-column: 2; grid-row: 2; font: var(--t-footnote); }
		.cs { grid-column: 3; grid-row: 1 / span 2; }
		.info { grid-template-columns: minmax(0, 1fr); gap: 20px; }
	}
	@media (max-width: 767px) {
		.person { --title-item: calc((100cqi - var(--shelf-gap, 12px)) / 1.6 / .78); gap: 26px; }
		.hero-in { grid-template-columns: 96px minmax(0, 1fr); }
		.grid { grid-template-columns: repeat(auto-fill, minmax(min(100%, 260px), 1fr)); }
		.tiles { grid-template-columns: repeat(3, minmax(0, 1fr)); }
	}
</style>
