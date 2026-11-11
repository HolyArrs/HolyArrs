<script lang="ts">
	import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
	import { createPersonArt } from '#lib/discover/index.ts';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import Poster from '#lib/ui/Poster.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import WideCard from '#lib/ui/WideCard.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import type { CardOwned } from '#lib/ui/CardBadges.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { followsByTmdb } from '#lib/ui/title-cards.ts';
	import { byDecade, personChips, runReviewed, type FilmSort, type FilmographyPage, type PersonMovie, type PersonRole } from '#lib/ui/people.ts';
	import { loadWikiFacts, type WikiFacts } from '#lib/ui/wiki-facts.ts';
	import ActionButton from '#lib/ui/ActionButton.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import HeroMotion from '#lib/ui/HeroMotion.svelte';
	import TrailerPlayer from '#lib/ui/TrailerPlayer.svelte';
	import { BrowseTile, peopleSort, takePrefetched } from '#lib/discover/index.ts';
	import { detailPath } from '#lib/metadata/index.ts';
	import type { AskLimit } from '#lib/requests/index.ts';
	import { paintWindow, sampleCover, tintOf, unpaintWindow } from '#lib/music/index.ts';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, message } from '#lib/core/api.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { to } from '#lib/core/paths.ts';
	import { plainReason } from '#lib/core/plain.ts';
	import { setTopCrumbs } from '#lib/core/top-crumbs.svelte.ts';
	import { minimalLook } from '#lib/core/look.svelte.ts';
	import { autoplayHere, loadUISettings, trailerSources, type TrailerSource } from '#lib/core/trailer/autoplay.svelte.ts';
	import { HERO_DELAY } from '#lib/core/trailer/trailer.ts';
	import { fromHandoff, openTheater } from '#lib/core/trailer/theater.svelte.ts';

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
	// The hero waits a short moment for the wiki facts (awards, bio), so its words do not move when they land.
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
			/* Private window: the choice lasts this visit. */
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

	const inLibrary: CardOwned = { state: 'available', get label() { return t('discover.people.inYourLibrary'); } };
	function owned(m: PersonMovie): CardOwned | null {
		if (m.title_id) return inLibrary;
		if (done[m.ids.tmdb] === 'Asked' || done[m.ids.tmdb] === 'Added') return { state: 'requested', label: t('discover.people.requested') };
		if (!m.ask || m.ask.state === 'turned_down') return null;
		return { state: 'requested', label: m.ask.state === 'approved' ? t('discover.people.approvedWaiting') : t('discover.people.requested') };
	}
	const href = (m: PersonMovie) => to(m.title_id ? `/library/${m.title_id}` : detailPath('movie', m.ids.tmdb));
	const roleLine = (m: PersonMovie) => {
		const who = m.characters?.length ? m.characters.join(' / ') : m.roles?.includes('Director') ? t('discover.people.director') : '';
		return [who, m.year ? String(m.year) : ''].filter(Boolean).join(' · ');
	};
	const personArt = createPersonArt();
	$effect(() => {
		for (const m of [...(data?.known_for ?? []), ...(data?.coming_soon ?? []), ...(data?.library ?? []), ...(data?.asked ?? []), ...movies]) personArt.probe(m);
	});
	const shownMovies = $derived(personArt.withArt(movies));
	const stats = $derived(data?.stats && role === 'all' && !busy ? { ...data.stats, films: shownMovies.length } : data?.stats);

	const hero = $derived(personArt.withArt(data?.known_for).find((m) => m.backdrop)?.backdrop ?? null);
	let heroIn = $state(false);
	$effect(() => {
		void hero;
		heroIn = false;
	});
	const chips = $derived(personChips(stats, personArt.withArt(data?.library).length, personArt.withArt(data?.asked).length));
	const factsAll = $derived.by((): { text: string; known: boolean }[] => {
		const p = data?.person;
		if (!p) return [];
		const known = data?.known_for?.[0]?.name;
		const born = p.birthday ? new Date(`${p.birthday}T00:00:00Z`).toLocaleDateString(appLocale(), { year: 'numeric', month: 'long', day: 'numeric', timeZone: 'UTC' }) : '';
		const died = wiki.died && wiki.died.length === 10 ? new Date(`${wiki.died}T00:00:00Z`).toLocaleDateString(appLocale(), { year: 'numeric', month: 'long', day: 'numeric', timeZone: 'UTC' }) : wiki.died ?? '';
		const from = p.birthplace || wiki.birth_place;
		return [
			{ text: known ? t('discover.people.knownForName', { name: known }) : (p.department ?? ''), known: !!known },
			{ text: born ? t('discover.people.bornOn', { date: born }) : '', known: false },
			{ text: from ? t('discover.people.fromPlace', { place: from }) : '', known: false },
			{ text: died ? t('discover.people.diedOn', { date: died }) : '', known: false }
		].filter((f) => f.text);
	});
	const facts = $derived(factsAll.map((f) => f.text));
	// The About sheet has three layouts (switch inside the sheet).
	let aboutView = $state<string>('profile');
	const longDate = (d?: string) => (d && d.length === 10 ? new Date(`${d}T00:00:00Z`).toLocaleDateString(appLocale(), { year: 'numeric', month: 'long', day: 'numeric', timeZone: 'UTC' }) : d ?? '');
	const about = $derived.by(() => {
		const p = data?.person;
		if (!p) return null;
		const born = p.birthday || wiki.born || '';
		const died = p.deathday || wiki.died || '';
		const end = died ? new Date(`${died}T00:00:00Z`) : new Date();
		const b = born.length === 10 ? new Date(`${born}T00:00:00Z`) : null;
		const age = b ? end.getUTCFullYear() - b.getUTCFullYear() - (end < new Date(Date.UTC(end.getUTCFullYear(), b.getUTCMonth(), b.getUTCDate())) ? 1 : 0) : 0;
		const rows = [
			{ k: t('discover.people.born'), v: longDate(born), sub: age && !died ? t('discover.people.yearsOld', { age }) : '' },
			{ k: t('discover.people.from'), v: p.birthplace || wiki.birth_place || '', sub: '' },
			{ k: t('discover.people.died'), v: longDate(died), sub: age && died ? t('discover.people.aged', { age }) : '' },
			{ k: t('discover.people.knownFor'), v: data?.known_for?.[0]?.name ?? '', sub: data?.known_for?.[0]?.year ? String(data.known_for[0].year) : '' },
			{ k: t('discover.people.work'), v: p.department ?? '', sub: yearsLine }
		].filter((x) => x.v);
		const wins = wiki.awards.filter((a) => a.won);
		const noms = wiki.awards.length - wins.length;
		const awards = [...wiki.awards].sort((x, y) => Number(y.won) - Number(x.won) || y.year - x.year).slice(0, 8);
		const paras = (bio || '').split(/\n+/).map((x) => x.trim()).filter(Boolean);
		const span = decades.map((g) => ({ label: g.label, n: g.movies.length }));
		const most = Math.max(1, ...span.map((x) => x.n));
		return { rows, wins: wins.length, noms, awards, paras, span, most, stills: aboutStills, wikiUrl: wiki.bio?.url ?? '' };
	});
	let still = $state(0);
	const minimal = $derived(minimalLook()); // The minimal look: no banner, no trailers, no colour from the art and plain About cards.
	$effect(() => {
		const n = about?.stills.length ?? 0;
		if (!view || minimal || n < 2 || matchMedia('(prefers-reduced-motion: reduce)').matches) return;
		const t = setInterval(() => (still = (still + 1) % n), 6000);
		return () => clearInterval(t);
	});
	const decades = $derived.by(() => {
		const g = byDecade(shownMovies);
		if (sort !== 'popular') return g;
		const soon = t('discover.people.comingSoon');
		return g.sort((a, b) => (a.label === soon ? 1 : b.label === soon ? -1 : b.label.localeCompare(a.label)));
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

	// Page layouts picked with ?look=1|2|3 (0 is the plain page). All three follow the music artist page (short banner fading into a tinted page, photo line, slim bar, figures, numbered list, About card), made for movies: 1 the Known-for still, 2 the Known-for trailers playing muted, 3 a slow reel of Known-for stills.
	const VIEWS = [{ value: 0, label: 'Now' }, { value: 1, label: 'A1 · Still' }, { value: 2, label: 'A2 · Trailers' }, { value: 3, label: 'A3 · Reel' }];
	const view: number = 2;
	const photo = $derived(data?.person.profile || wiki.image || '');
	const yearsLine = $derived.by(() => {
		const st = data?.stats;
		return st?.first_year ? `${st.first_year}–${st.active ? t('discover.people.now') : st.last_year}` : '';
	});
	const lead = $derived([data?.person.department, ...factsAll.filter((f) => !f.known).map((f) => f.text)].filter(Boolean).join(' · '));
	const top = $derived(personArt.withArt(data?.known_for).slice(0, 10));
	// Under the name: only what they do and since when; the rest lives in About.
	const roleWords = $derived.by(() => {
		const all = [...(data?.known_for ?? []), ...movies].flatMap((m) => m.roles ?? []);
		const acts = all.includes('Actor'), directs = all.includes('Director');
		const dept = data?.person.department;
		const order = dept === 'Directing' ? ['Director', 'Actor'] : ['Actor', 'Director'];
		const got = order.filter((w) => (w === 'Actor' ? acts : directs)).map((w) => (w === 'Actor' ? t('discover.people.actor') : t('discover.people.director')));
		return got.length ? got.join(' · ') : dept === 'Acting' ? t('discover.people.actor') : dept ?? '';
	});
	const quiet = $derived([roleWords, data?.stats?.first_year ? t('discover.people.since', { year: data.stats.first_year }) : ''].filter(Boolean).join(' · '));
	const reel = $derived(personArt.withArt(data?.known_for).map((m) => m.backdrop).filter((b): b is string => !!b).slice(0, 6));
	// About starts half way through the stills (more of them when there are), so it never shows the banner's picture.
	const aboutStills = $derived.by(() => {
		const all = personArt.withArt(data?.known_for).map((m) => m.backdrop).filter((b): b is string => !!b).slice(0, 12);
		const rest = all.slice(reel.length);
		if (rest.length) return rest;
		const h = Math.ceil(reel.length / 2);
		return reel.length > 1 ? [...reel.slice(h), ...reel.slice(0, h)] : reel;
	});
	let aboutOpen = $state(false);
	let allDecade = $state<{ label: string; movies: PersonMovie[] } | null>(null);

	// The banner zooms out and darkens as it leaves; the slim bar shows once the name has gone.
	let heroBox: HTMLElement | undefined = $state();
	let nameEl: HTMLElement | undefined = $state();
	let gone = $state(0);
	let slimOn = $state(false);
	// The top bar and the slim bar are one solid band in the person's colour.
	$effect(() => {
		if (minimal) return;
		const c = document.documentElement.classList;
		c.add('pbar');
		return () => c.remove('pbar');
	});
	function measure() {
		if (heroBox) {
			const r = heroBox.getBoundingClientRect();
			gone = Math.min(1, Math.max(0, -r.top / Math.max(1, r.height)));
		}
		const bar = document.querySelector('.ui-topbar');
		const barBottom = bar && !document.documentElement.classList.contains('bar-hidden') ? Math.max(0, bar.getBoundingClientRect().bottom) : 0;
		slimOn = !!nameEl && nameEl.getBoundingClientRect().bottom < barBottom + 4;
	}
	$effect(() => {
		if (!view) return;
		measure();
		addEventListener('scroll', measure, { passive: true, capture: true });
		return () => removeEventListener('scroll', measure, { capture: true });
	});

	// The page takes its colour from the person's photo (the banner when there is none), as artist pages do.
	let sampled = $state<string | null>(null);
	$effect(() => {
		const src = photo || hero;
		if (!view || minimal || !src || sampled) return;
		const img = new Image();
		img.onload = () => (sampled = sampleCover(img));
		img.src = src;
	});
	const tint = $derived(view && !minimal && sampled ? tintOf(sampled) : null);
	$effect(() => {
		if (tint) {
			paintWindow(tint);
			return () => unpaintWindow();
		}
	});

	// The Known-for trailers play muted in the banner, one after another, after the usual dwell.
	let clip = $state(0);
	let sources = $state<TrailerSource[]>([]);
	let clipOn = $state(false);
	// One trailer, then the banner settles back on the turning stills.
	let clipDone = $state(false);
	// Trailers open on studio cards and logos; the banner joins a few seconds in.
	const TRAILER_SKIP = 8;
	// Paused: the stills come back under the buttons.
	let clipHeld = $state(false);
	$effect(() => {
		const m = top[clip % Math.max(1, top.length)];
		sources = [];
		clipOn = false;
		if (view !== 2 || minimal || !m || clipDone) return;
		const ctl = new AbortController();
		const t = setTimeout(() => {
			if (!autoplayHere()) return;
			void trailerSources('movie', m.ids.tmdb, undefined, ctl.signal).then((s) => !ctl.signal.aborted && (sources = s)).catch(() => {});
		}, HERO_DELAY);
		return () => (clearTimeout(t), ctl.abort());
	});
	// A trailer that will not play moves to the next; once every one failed, the banner settles on the stills (it never goes round again).
	let clipsFailed = 0;
	$effect(() => {
		void top;
		clipsFailed = 0;
	});
	const nextClip = () => {
		clipsFailed++;
		if (top.length <= 1 || clipsFailed >= top.length) {
			clipDone = true;
			return;
		}
		clip = (clip + 1) % top.length;
	};
	const nowShowing = $derived(top[clip % Math.max(1, top.length)]);
	onMount(() => void loadUISettings());

	// Where this person was opened from is in the top bar (layout), like every page.
	setTopCrumbs(null);
</script>

<svelte:head><title>{data?.person.name ?? t('discover.people.person')} · HolyArrs</title>{#if hero && !minimal}<link rel="preload" as="image" href={hero} />{/if}</svelte:head>

{#snippet filmCard(m: PersonMovie)}
	<WideCard cover title={m.name} href={href(m)} image={m.backdrop || m.poster} kind="movie" subtitle={roleLine(m) || undefined} owned={owned(m)} preview={{ kind: 'movie', tmdb: m.ids.tmdb, year: m.year || undefined, titleId: m.title_id }}
		followTmdb={!m.title_id && (coming.has(m.ids.tmdb) || followsByTmdb({ kind: 'movie', titleId: m.title_id, year: m.year }))}>
		{#snippet growAction()}
			{#if action && !m.title_id && !m.ask && !m.never_add && !done[m.ids.tmdb]}
				<IconButton icon="plus" label={action === 'add' ? t('discover.people.add', { name: m.name }) : t('discover.people.askFor', { name: m.name })} variant="glass" size="lg" aria-busy={asking === m.ids.tmdb || undefined} onclick={() => quick(m)} />
			{/if}
		{/snippet}
	</WideCard>
{/snippet}

{#snippet row(title: string, list: PersonMovie[] | undefined, anchor: string)}
	{#if personArt.withArt(list).length}
		<Shelf {title} {anchor} item="var(--title-item)" items={personArt.withArt(list)} key={(m) => m.ids.tmdb} placeholder="wide" lines={1}>
			{#snippet card(m: PersonMovie)}{@render filmCard(m)}{/snippet}
		</Shelf>
	{/if}
{/snippet}

<div class="person">
{#if error && !data}<LoadError hasData={false} message={plainReason(error).text} onretry={load} {busy} />
{:else if error}<p class="notice bad" role="alert"><Icon name="alert" size={17} /><span>{plainReason(error).text}</span></p>{/if}
{#snippet mainAct()}
	<ActionButton icon="film" label={t('discover.people.goToTheFilmography')} caption={t('discover.people.filmography')} primary onclick={() => document.getElementById('films-title')?.scrollIntoView({ behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth', block: 'start' })} />
{/snippet}

<PageLoader loading={(!data || !(wikiDone || wikiWaited)) && !error} label={t('discover.people.loadingThisPerson')} onretry={load}>
{#if data && (wikiDone || wikiWaited) && view}
	<div class="ap" class:tinted={!!tint} class:minimal style={tint ? `--ap-bg:${tint.bg};--ap-soft:${tint.soft}` : undefined}>
	{#if !minimal}<header class="banner" bind:this={heroBox} style:--gone={gone.toFixed(3)} class:playing={clipOn} class:held={clipHeld} data-hero data-testid="person-banner">
		<div class="stage" aria-hidden="true">
			{#if reel.length}
				<HeroMotion pictures={reel} paused={clipOn && !clipHeld} position="center 25%" onload={() => (heroIn = true)} />
			{:else if hero}
				<img class="art" class:in={heroIn} src={hero} alt="" decoding="async" fetchpriority="high" onload={() => (heroIn = true)} />
			{/if}
		</div>
		{#if view === 2 && !clipDone && sources.length && nowShowing}
			{#key nowShowing.ids.tmdb}
				<div class="clip"><TrailerPlayer {sources} title={nowShowing.name} sharp pausable top start={TRAILER_SKIP} onexpand={(o) => nowShowing && openTheater({ kind: 'movie', tmdb: nowShowing.ids.tmdb, title: nowShowing.name, ...fromHandoff(o), from: 'hero' })} onplaying={() => (clipOn = true)} onpause={(v) => (clipHeld = v)} onended={() => ((clipOn = false), (clipDone = true))} onfail={() => ((clipOn = false), nextClip())} /></div>
			{/key}
		{/if}
		<div class="b-shade" aria-hidden="true"></div>
		<div class="dim" aria-hidden="true"></div>
		{#if view === 2 && clipOn && nowShowing}<a class="showing" href={href(nowShowing)}>{nowShowing.name}{nowShowing.year ? ` · ${nowShowing.year}` : ''}</a>{/if}
	</header>{/if}

	<div class="cline">
		<span class="photo">{#if photo}<img src={photo} alt="" fetchpriority="high" />{:else}<span class="initials">{data.person.name.split(' ').map((w) => w[0]).slice(0, 2).join('')}</span>{/if}</span>
		<div class="c-words">
			<h1 bind:this={nameEl}>{data.person.name}</h1>
			{#if quiet}<p class="c-line">{quiet}</p>{/if}
		</div>
		<div class="acts">{@render mainAct()}<ActionButton icon="info" label={t('discover.people.about', { name: data.person.name })} caption={t('discover.people.about2')} onclick={() => (aboutOpen = true)} /></div>
	</div>

	<div class="slimwrap">
		<div class="slim" class:on={slimOn} style:--tint={tint?.bg} aria-hidden={!slimOn} inert={!slimOn} data-testid="person-slim-bar">
			{@render mainAct()}
			{#if photo}<img class="s-photo" src={photo} alt="" />{/if}
			<span class="slim-name">{data.person.name}</span>
		</div>
	</div>

	{#if limit && action === 'ask' && !limit.no_limit && !limit.movies.no_limit}
		<p class="note">{t('discover.people.movieAsksThisRolling', { used: limit.movies.used, left: limit.movies.left })}</p>
	{/if}

	{#if top.length}
		<section class="block kf kf-eps" data-testid="person-top">
			<h2>{t('discover.people.knownFor')}</h2>
			<ol class="kf-list">
				{#each top as m, i (m.ids.tmdb)}
					<li class="kf-item" style:--i={i}>
						<span class="kf-pic">{@render filmCard(m)}</span>
						<span class="kf-n" aria-hidden="true">{i + 1}</span>
					</li>
				{/each}
			</ol>
		</section>
	{/if}

	{@render row(t('discover.people.comingSoon'), data.coming_soon, 'person-soon')}
	{@render row(t('discover.people.inYourLibrary'), data.library, 'person-library')}
	{@render row(t('discover.people.youAskedFor'), data.asked, 'person-asked')}

	<section class="block" aria-labelledby="films-title">
		<div class="toolbar">
			<h2 id="films-title">{t('discover.people.filmography')}</h2>
			<div class="controls">
				<SegmentedControl label={t('discover.people.creditRole')} value={role} options={[{ value: 'all', label: t('discover.people.all') }, { value: 'actor', label: t('discover.people.actor') }, { value: 'director', label: t('discover.people.director') }]} onchange={(v) => changeRole(v as PersonRole)} />
				<SegmentedControl label={t('discover.people.sort')} value={sort} options={[{ value: 'oldest', label: t('discover.people.oldestFirst') }, { value: 'newest', label: t('discover.people.newestFirst') }, { value: 'popular', label: t('discover.people.popular') }]} onchange={(v) => changeSort(v as FilmSort)} />
			</div>
		</div>
		{#if !shownMovies.length && !busy}<EmptyState compact icon="film" title={t('discover.people.noMoviesMatchThis')} />{/if}
	</section>
	{#each decades as g (g.label)}
		<Shelf title={g.label} item="var(--title-item)" items={g.movies.slice(0, 12)} key={(m) => m.ids.tmdb} placeholder="wide" lines={1} onseeall={g.movies.length > 12 ? () => (allDecade = g) : undefined}>
			{#snippet card(m: PersonMovie)}{@render filmCard(m)}{/snippet}
		</Shelf>
	{/each}

	{#if bio || facts.length}
		<section class="block">
			<h2>{t('discover.people.about2')}</h2>
			<button type="button" class="acard" class:minimal aria-haspopup="dialog" aria-label={t('discover.people.aboutReadMore', { name: data.person.name })} onclick={() => (aboutOpen = true)} onpointermove={(e) => { const b = e.currentTarget.getBoundingClientRect(); e.currentTarget.style.setProperty('--mx', `${((e.clientX - b.left) / b.width) * 100}%`); e.currentTarget.style.setProperty('--my', `${((e.clientY - b.top) / b.height) * 100}%`); }} data-testid="person-about">
				{#if !minimal}{#each about?.stills ?? [] as src, i (src)}<img class="ab-frame" class:on={i === still % (about?.stills.length || 1)} {src} alt="" loading="lazy" decoding="async" />{/each}<span class="a-glow" aria-hidden="true"></span>{/if}
				{#if wiki.badges[0]}<span class="disc" aria-hidden="true"><b>{wiki.badges[0].split(' ')[0]}</b><small>{wiki.badges[0].split(' ').slice(1).join(' ')}</small></span>{/if}
				<span class="a-foot">
					<span class="a-lis">{[stats?.films ? t('discover.people.filmsCount', { n: stats.films }) : '', yearsLine].filter(Boolean).join(' · ')}</span>
					<span class="a-bio">{bio || data.person.name}</span>
					<span class="a-more">{t('discover.people.moreAbout', { name: data.person.name.split(' ')[0] })}</span>
				</span>
			</button>
		</section>
		<Sheet bind:open={aboutOpen} title={t('discover.people.about', { name: data.person.name })} size="lg">
			{#if about}
			<div class="ab ab-{aboutView}" data-testid="person-about-sheet">
				{#if aboutView !== 'dossier' && (minimal || about.stills.length)}
					<figure class="ab-still" class:plain={minimal}>
						{#if !minimal}{#each about.stills as src, i (src)}<img class="ab-frame" class:on={i === still % about.stills.length} {src} alt="" decoding="async" loading={i ? 'lazy' : 'eager'} />{/each}{/if}
						<figcaption>
							{#if photo}<img class="ab-face" src={photo} alt="" />{/if}
							<span><b>{data.person.name}</b>{#if lead}<small>{lead}</small>{/if}</span>
						</figcaption>
					</figure>
				{/if}

				{#if aboutView === 'career'}
					<div class="ab-figs">
						{#if stats?.films}<p><b>{stats.films}</b><span>{t('discover.people.films')}</span></p>{/if}
						{#if data.stats?.shows}<p><b>{data.stats.shows}</b><span>{t('discover.people.shows')}</span></p>{/if}
						{#if yearsLine}<p><b>{yearsLine}</b><span>{t('discover.people.onScreen')}</span></p>{/if}
						{#if about.wins || about.noms}<p><b>{about.wins}</b><span>{about.wins === 1 ? t('discover.people.winNoms', { n: about.noms }) : t('discover.people.winsNoms', { n: about.noms })}</span></p>{/if}
					</div>
					{#if about.span.length > 1}
						<section class="ab-sec"><h3>{t('discover.people.byDecade')}</h3>
							<ol class="ab-bars">{#each [...about.span].reverse() as d (d.label)}<li><span class="bar" style:--h={d.n / about.most}><i></i></span><b>{d.n}</b><small>{d.label}</small></li>{/each}</ol>
						</section>
					{/if}
				{/if}

				<div class="ab-body">
					{#if aboutView === 'dossier'}
						<aside class="ab-side">
							<span class="ab-round">{#if photo}<img src={photo} alt="" />{/if}</span>
							<h3 class="ab-name">{data.person.name}</h3>
							<dl class="ab-dl">{#each about.rows as x (x.k)}<div><dt>{x.k}</dt><dd>{x.v}{#if x.sub}<small>{x.sub}</small>{/if}</dd></div>{/each}</dl>
						</aside>
					{:else if aboutView === 'profile'}
						<dl class="ab-tiles">{#each about.rows as x (x.k)}<div><dt>{x.k}</dt><dd>{x.v}</dd>{#if x.sub}<small>{x.sub}</small>{/if}</div>{/each}</dl>
					{/if}
					<div class="ab-main">
						{#if about.paras.length}
							<section class="ab-sec"><h3>{t('discover.people.story')}</h3>
								<div class="ab-bio">{#each about.paras as t, i (i)}<p class:lead={i === 0}>{t}</p>{/each}</div>
							</section>
						{/if}
						{#if about.awards.length}
							<section class="ab-sec"><h3>{t('discover.people.awards')}</h3>
								<ul class="ab-awards">{#each about.awards as a, i (i)}<li class:won={a.won}><span class="ab-yr">{a.year || ''}</span><span class="ab-aw"><b>{a.name}</b>{#if a.for_work}<small>{a.for_work}</small>{/if}</span><span class="ab-tag">{a.won ? t('discover.people.won') : t('discover.people.nominated')}</span></li>{/each}</ul>
							</section>
						{/if}
						{#if aboutView === 'career' && about.rows.length}
							<section class="ab-sec"><h3>{t('discover.people.details')}</h3><dl class="ab-dl">{#each about.rows as x (x.k)}<div><dt>{x.k}</dt><dd>{x.v}{#if x.sub}<small>{x.sub}</small>{/if}</dd></div>{/each}</dl></section>
						{/if}
						{#if about.wikiUrl}<a class="ab-src" href={about.wikiUrl} target="_blank" rel="noopener noreferrer">{t('discover.people.readMoreOnWikipedia')}</a>{/if}
					</div>
				</div>
			</div>
			{/if}
		</Sheet>
	{/if}

	{#if data.often_with?.length}
		<section class="block">
			<h2>{t('discover.people.oftenWorksWith')}</h2>
			<ul class="circles">{#each data.often_with as p (p.id)}<li><a class="mate" href={to(`/people/${p.id.replace(/^tmdb:/, '')}`)}><span class="mate-pic">{#if p.profile}<img src={p.profile} alt="" loading="lazy" decoding="async" />{:else}<span class="initials">{p.name.split(' ').map((w) => w[0]).slice(0, 2).join('')}</span>{/if}</span><span class="mate-name">{p.name}</span></a></li>{/each}</ul>
		</section>
	{/if}

	{#if allDecade}
		<Sheet open={true} onclose={() => (allDecade = null)} title={allDecade.label} size="lg">
			<ul class="grid">{#each allDecade.movies as m (m.ids.tmdb)}<li>{@render filmCard(m)}</li>{/each}</ul>
		</Sheet>
	{/if}
	</div>
{:else if data && (wikiDone || wikiWaited)}
	<header class="hero" data-hero>
		{#if hero}<img class="art" class:in={heroIn} src={hero} alt="" decoding="async" fetchpriority="high" onload={() => (heroIn = true)} onerror={() => (heroIn = false)} />{/if}
		<div class="shade" aria-hidden="true"></div>
		<div class="hero-in">
			<div class="portrait"><Poster src={data.person.profile || wiki.image} name={data.person.name} /></div>
			<div class="who">
				<h1>{data.person.name}</h1>
				{#if facts.length}<p class="facts">{facts.join(' · ')}</p>{/if}
				{#if chips.length || wiki.badges.length}<ul class="chips" aria-label={t('discover.people.atAGlance')}>{#each wiki.badges.slice(0, 3) as c (c)}<li class="award" data-testid="award-badge">{c}</li>{/each}{#each chips as c (c)}<li>{c}</li>{/each}</ul>{/if}
				{#if bio}
					<p class="bio" class:open={bioOpen}>{bio}</p>
					<button class="more" type="button" onclick={() => (bioOpen = !bioOpen)}>{bioOpen ? t('discover.people.less') : t('discover.people.more')}</button>
				{/if}
			</div>
		</div>
	</header>

	{#if limit && action === 'ask' && !limit.no_limit && !limit.movies.no_limit}
		<p class="note">{t('discover.people.movieAsksThisRolling', { used: limit.movies.used, left: limit.movies.left })}</p>
	{/if}

	{@render row(t('discover.people.comingSoon'), data.coming_soon, 'person-soon')}
	{@render row(t('discover.people.knownFor'), data.known_for, 'person-known')}
	{@render row(t('discover.people.inYourLibrary'), data.library, 'person-library')}
	{@render row(t('discover.people.youAskedFor'), data.asked, 'person-asked')}

	{#if data.often_with?.length}
		<section class="block" aria-labelledby="with-title">
			<h2 id="with-title">{t('discover.people.oftenWorksWith')}</h2>
			<ul class="tiles">
				{#each data.often_with as p (p.id)}
					<li><BrowseTile name={p.name} href={to(`/people/${p.id.replace(/^tmdb:/, '')}`)} image={p.profile ?? ''} hue="#555b66" plain /></li>
				{/each}
			</ul>
		</section>
	{/if}

	<section class="block" aria-labelledby="films-title">
		<div class="toolbar">
			<h2 id="films-title">{t('discover.people.filmography')}</h2>
			<div class="controls">
				<SegmentedControl label={t('discover.people.creditRole')} value={role} options={[{ value: 'all', label: t('discover.people.all') }, { value: 'actor', label: t('discover.people.actor') }, { value: 'director', label: t('discover.people.director') }]} onchange={(v) => changeRole(v as PersonRole)} />
				<SegmentedControl label={t('discover.people.sort')} value={sort} options={[{ value: 'oldest', label: t('discover.people.oldestFirst') }, { value: 'newest', label: t('discover.people.newestFirst') }, { value: 'popular', label: t('discover.people.popular') }]} onchange={(v) => changeSort(v as FilmSort)} />
			</div>
		</div>
		{#each decades as g (g.label)}
			<h3 class="decade">{g.label}</h3>
			<ul class="grid" aria-busy={busy || undefined}>
				{#each g.movies as m (m.ids.tmdb)}<li>{@render filmCard(m)}</li>{/each}
			</ul>
		{/each}
		{#if !shownMovies.length && !busy}<EmptyState compact icon="film" title={t('discover.people.noMoviesMatchThis')} />{/if}
	</section>

{/if}
</PageLoader>
</div>

<style>
	.person { --title-item: max(200px, calc((100vw - 96px) / 5.16)); display: flex; flex-direction: column; gap: 32px; padding-bottom: 48px; min-width: 0; }
	/* The hero stays dark in both themes, like the title pages. */
	.hero { position: relative; isolation: isolate; min-height: clamp(380px, 38vw, 560px); display: flex; align-items: flex-end; overflow: hidden; margin: calc(var(--s-6) * -1) calc(var(--gutter) * -1) 0; background: var(--on-art-ink); color: var(--on-art); }
	.art { position: absolute; inset: 0; z-index: -2; width: 100%; height: 100%; object-fit: cover; object-position: center 25%; }
	/* The hero image fades in over 400 ms once loaded. */
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
	.ap { --gap: 40px; display: flex; flex-direction: column; gap: var(--gap); min-width: 0; }
	.ap.tinted { --text: #fff; --bg: var(--ap-bg); --text-muted: var(--ap-soft); --surface-1: rgb(255 255 255 / 8%); --surface-2: rgb(255 255 255 / 12%); --border: rgb(255 255 255 / 14%); --line: rgb(255 255 255 / 14%); color: #fff; color-scheme: dark; }
	.ap h1 { font-family: var(--font-display); font-weight: 800; line-height: 1; letter-spacing: -.03em; text-wrap: balance; overflow-wrap: anywhere; }
	.banner { --ink: var(--ap-bg, #07080a); position: relative; isolation: isolate; overflow: hidden; height: clamp(280px, 44vh, 520px); margin: calc(var(--s-6) * -1) calc(var(--gutter) * -1) 0; background: var(--ink); color: #fff; -webkit-mask-image: linear-gradient(to bottom, #000 55%, rgb(0 0 0 / .82) 68%, rgb(0 0 0 / .5) 80%, rgb(0 0 0 / .18) 91%, transparent 100%); mask-image: linear-gradient(to bottom, #000 55%, rgb(0 0 0 / .82) 68%, rgb(0 0 0 / .5) 80%, rgb(0 0 0 / .18) 91%, transparent 100%); }
	.stage { position: absolute; inset: 0; z-index: -2; scale: calc(1.1 - .1 * var(--gone, 0)); transform-origin: 50% 40%; will-change: scale; }
	.stage .art { width: 100%; height: 100%; object-fit: cover; object-position: center 25%; opacity: 0; transition: opacity var(--dur-slow) var(--ease-out); }
	.stage .art.in { opacity: .85; }
	.clip { position: absolute; inset: 0; z-index: 2; opacity: 0; transition: opacity var(--dur-slow) var(--ease-out); }
	.banner.playing .clip { opacity: 1; }
	.clip :global(video), .clip :global(iframe), .clip::after { transition: opacity var(--dur-slow) var(--ease-out); }
	.banner.held .clip :global(video), .banner.held .clip :global(iframe), .banner.held .clip::after { opacity: 0; }
	/* The trailer sits above the banner shades, so it carries its own fade; its buttons go top right like the artist page. */
	.clip::after { content: ''; position: absolute; inset: 0; z-index: 1; pointer-events: none; background: linear-gradient(to top, color-mix(in srgb, var(--ink) 60%, transparent) 0%, transparent 45%), linear-gradient(to bottom, rgb(7 8 10 / .35), transparent 25%); }
	.clip :global(video), .clip :global(iframe) { width: 100%; height: 100%; object-fit: cover; }
	.b-shade { position: absolute; inset: 0; z-index: 0; pointer-events: none; background: linear-gradient(to top, color-mix(in srgb, var(--ink) 60%, transparent) 0%, color-mix(in srgb, var(--ink) 25%, transparent) 35%, transparent 60%), linear-gradient(to bottom, rgb(7 8 10 / .4), rgb(7 8 10 / 0) 30%); }
	.dim { position: absolute; inset: 0; z-index: 0; background: var(--ink); opacity: calc(var(--gone, 0) * .85); pointer-events: none; }
	.showing { position: absolute; z-index: 3; top: 18px; left: var(--gutter); padding: 6px 12px; border-radius: 999px; background: rgb(255 255 255 / 14%); border: 1px solid rgb(255 255 255 / 18%); backdrop-filter: blur(18px) saturate(160%); color: #fff; font: var(--t-footnote); font-weight: 600; text-decoration: none; }
	@media (prefers-reduced-motion: reduce) { .stage { scale: 1; } }
	.cline { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: end; gap: 20px; margin-top: calc(var(--gap) * -1 - 52px); position: relative; z-index: 1; }
	.photo { display: grid; place-items: center; width: 150px; aspect-ratio: 2 / 3; height: auto; border-radius: var(--r-card); overflow: hidden; background: linear-gradient(160deg, var(--surface-2), var(--surface-3, var(--surface-2))); box-shadow: var(--shadow-card), 0 18px 40px rgb(0 0 0 / .35); transition: transform var(--dur) var(--ease-out); }
	.photo:hover { transform: scale(1.02); }
	.photo img { width: 100%; height: 100%; object-fit: cover; object-position: center 20%; }
	.initials { color: #fff; font: 700 40px/1 var(--font-display); }
	.c-words { display: flex; flex-direction: column; gap: 4px; min-width: 0; padding-bottom: 8px; }
	.c-words h1 { margin: 0; font-size: clamp(30px, 3vw, 40px); color: var(--text); }
	.c-line { margin: 0; color: var(--text-muted); font: 500 17px/1.3 var(--font-ui); letter-spacing: -.005em; font-variant-numeric: tabular-nums; }
	.acts { display: flex; align-items: center; gap: 10px; padding-bottom: 8px; }
	.go { display: inline-flex; align-items: center; height: 44px; padding: 0 20px; border-radius: 999px; background: var(--text); color: var(--bg); font: 600 15px/1 var(--font-ui); text-decoration: none; }
	.go:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.slimwrap { position: sticky; top: var(--topbar); z-index: 25; height: 0; margin-bottom: calc(var(--gap) * -1); transition: transform var(--bar-glide); }
	:global(html.bar-hidden) .slimwrap { transform: translateY(calc(-1 * var(--topbar))); }
	.slim { --tint: #3a3d44; position: absolute; top: 0; left: calc(var(--gutter) * -1); right: calc(var(--gutter) * -1); display: flex; align-items: center; gap: 14px; height: 64px; padding: 0 var(--gutter); box-sizing: border-box; background: color-mix(in srgb, var(--tint) 45%, #0b0c0f); color: #fff; box-shadow: 0 8px 24px rgb(0 0 0 / .22); opacity: 0; translate: 0 -8px; pointer-events: none; transition: opacity var(--dur) var(--ease-out), translate var(--dur) var(--ease-out); }
	/* The top bar and the slim bar read as one solid band in the person's colour. */
	:global(html.pbar .ui-topbar), :global(html.pbar[data-tint] .ui-topbar) { background-color: var(--tint-bg, #1b1c20); border-bottom-color: transparent; -webkit-backdrop-filter: none; backdrop-filter: none; }
	:global(html.pbar) .slim { background: var(--tint-bg, #1b1c20); box-shadow: 0 10px 24px rgb(0 0 0 / .3); }
	/* Minimal: no banner, so the photo row is a plain header under the top bar, and the bars take the theme. */ .ap.minimal .cline { margin-top: 0; padding-top: 8px; } .ap.minimal .photo { box-shadow: var(--shadow-card); } .ap.minimal .initials { color: var(--text-muted); } .ap.minimal .slim { background: var(--bg); color: var(--text); border-bottom: 1px solid var(--line); box-shadow: none; } :global(html:not([data-theme='dark'])) .ap.minimal .acts :global(.ui-act) { background: color-mix(in srgb, var(--text) 7%, transparent); border-color: var(--line); color: var(--text); backdrop-filter: none; -webkit-backdrop-filter: none; } :global(html:not([data-theme='dark'])) .ap.minimal .acts :global(.ui-act.primary), :global(html:not([data-theme='dark'])) .ap.minimal .slim :global(.ui-act.primary) { background: var(--text); border-color: var(--text); color: var(--bg); box-shadow: none; } :global(html:not([data-theme='dark'])) .ap.minimal .acts :global(.ui-act:focus-visible) { box-shadow: 0 0 0 2px var(--bg), 0 0 0 4px var(--text); }
	.slim.on { opacity: 1; translate: 0 0; pointer-events: auto; }
	.s-photo { width: 32px; height: 48px; border-radius: 6px; object-fit: cover; object-position: center 20%; }
	.slim-name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: 700 22px/1.1 var(--font-display); letter-spacing: -.01em; }
	@media (prefers-reduced-motion: reduce) { .slim, .slimwrap { transition: none; translate: none; } }
	.own { display: inline-flex; align-items: center; gap: 6px; }
	.award { color: #f5c518; }
	.songs { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); grid-template-rows: repeat(5, auto); grid-auto-flow: column; gap: 0 24px; margin: 0; padding: 0; list-style: none; }
	.song { display: grid; grid-template-columns: 32px 48px minmax(0, 1fr) auto; align-items: center; gap: 12px; height: 80px; padding: 0 8px; border-radius: 10px; transition: background var(--dur-fast) var(--ease-out); }
	.song:hover { background: var(--surface-1); }
	.num { display: grid; place-items: center; width: 32px; color: var(--text-muted); font: 600 15px/1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.thumb { display: block; width: 48px; height: 72px; border-radius: 6px; overflow: hidden; background: var(--surface-2); }
	.thumb img { width: 100%; height: 100%; object-fit: cover; }
	.s-words { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
	.s-name { color: var(--text); font: 600 15px/1.3 var(--font-ui); text-decoration: none; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.s-name:hover { text-decoration: underline; }
	.s-sub { color: var(--text-muted); font: var(--t-footnote); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.state { color: var(--text-muted); font: var(--t-footnote); font-weight: 600; }
	.state.have { color: #9df5b4; }
	.acard { position: relative; display: block; width: 100%; height: clamp(280px, 30vw, 420px); padding: 0; border: 0; border-radius: var(--r-card); overflow: hidden; background: #1c1d21; color: #fff; text-align: left; cursor: pointer; isolation: isolate; box-shadow: var(--shadow-card); transition: translate var(--dur-slow) var(--ease-out); }
	.acard { box-shadow: 0 0 0 1px rgb(255 255 255 / 8%), 0 24px 60px rgb(0 0 0 / .4); }
	.acard::after { content: ''; position: absolute; inset: 0; z-index: -1; background: linear-gradient(to top, color-mix(in srgb, var(--ap-bg, #000) 70%, #000) 0%, color-mix(in srgb, var(--ap-bg, #000) 35%, transparent) 45%, transparent 75%), linear-gradient(to right, rgb(0 0 0 / .35), transparent 60%); }
	.a-glow { position: absolute; inset: 0; z-index: -1; pointer-events: none; background: radial-gradient(60% 50% at var(--mx, 70%) var(--my, 20%), rgb(255 255 255 / .1), transparent 70%); opacity: 0; transition: opacity var(--dur) var(--ease-out); }
	.acard:hover .a-glow { opacity: 1; }
	.a-more { align-self: flex-start; margin-top: 6px; padding: 9px 16px; border-radius: 999px; background: rgb(255 255 255 / 14%); box-shadow: inset 0 0 0 1px rgb(255 255 255 / 16%); backdrop-filter: blur(20px) saturate(160%); font: 600 14px/1 var(--font-ui); transition: background var(--dur-fast) var(--ease-out); }
	.acard:hover .a-more { background: rgb(255 255 255 / 24%); }
	.acard:hover { translate: 0 -2px; }
	.acard:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.disc { position: absolute; top: 20px; left: 20px; display: grid; place-content: center; justify-items: center; width: 92px; height: 92px; border-radius: 50%; background: #f5c518; color: #111; box-shadow: 0 8px 24px rgb(0 0 0 / .3); text-align: center; }
	.disc b { font: 800 26px/1 var(--font-display); }
	.disc small { font: 600 11px/1.1 var(--font-ui); max-width: 70px; }
	.a-foot { position: absolute; left: 28px; right: 28px; bottom: 26px; display: flex; flex-direction: column; gap: 8px; }
	.a-lis { font: 700 13px/1 var(--font-ui); text-transform: uppercase; letter-spacing: .08em; opacity: .8; }
	.a-bio { font: 500 19px/1.45 var(--font-ui); color: rgb(255 255 255 / .9); text-shadow: 0 1px 12px rgb(0 0 0 / .35); display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; max-width: 72ch; }
	/* Minimal: a plain card in the theme's colours, the text in the flow. */ .acard.minimal { display: flex; align-items: center; gap: 20px; height: auto; padding: 20px; background: var(--surface-1); color: var(--text); box-shadow: 0 0 0 1px var(--line); } .acard.minimal::after { content: none; } .acard.minimal:focus-visible { box-shadow: var(--focus-ring); } .acard.minimal .disc { position: static; flex: none; box-shadow: none; } .acard.minimal .a-foot { position: static; min-width: 0; } .acard.minimal .a-bio { color: var(--text); text-shadow: none; } .acard.minimal .a-more { background: var(--surface-2); box-shadow: inset 0 0 0 1px var(--line); backdrop-filter: none; -webkit-backdrop-filter: none; } .acard.minimal:hover .a-more { background: var(--line); }
	.s-facts { margin: 0 0 12px; color: var(--text-muted); font: var(--t-subhead); }
	.s-bio { margin: 0; font: var(--t-body); line-height: 1.6; white-space: pre-line; }
	.circles { display: flex; gap: 20px; overflow-x: auto; margin: 0; padding: 4px 2px 8px; list-style: none; scrollbar-width: none; }
	.circles li { flex: none; }
	.mate { display: flex; flex-direction: column; gap: 8px; width: 132px; color: var(--text); text-decoration: none; }
	.mate-pic { display: grid; place-items: center; aspect-ratio: 2 / 3; border-radius: var(--r-card); overflow: hidden; background: linear-gradient(160deg, var(--surface-2), var(--surface-3, var(--surface-2))); box-shadow: var(--shadow-card); transition: transform var(--dur) var(--ease-out); }
	.mate-pic img { width: 100%; height: 100%; object-fit: cover; object-position: center 20%; }
	.mate:hover .mate-pic { transform: translateY(-2px) scale(1.02); }
	.mate:focus-visible { outline: none; } .mate:focus-visible .mate-pic { box-shadow: var(--focus-ring); }
	.mate-name { font: 600 14px/1.25 var(--font-ui); text-align: center; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	@media (prefers-reduced-motion: reduce) { .mate:hover .mate-pic { transform: none; } }
	@media (max-width: 1000px) { .songs { grid-template-columns: minmax(0, 1fr); grid-template-rows: none; grid-auto-flow: row; } }
	/* Known for: wide stills with a numbered line, like an episode list. */
	.kf-list { margin: 0; padding: 0; list-style: none; }
	.kf-item { position: relative; min-width: 0; animation: ab-in var(--dur-slow) var(--ease-out) both; animation-delay: calc(var(--i) * 35ms); }
	.kf-pic { position: relative; display: block; min-width: 0; }
	.kf-pic :global(.ui-wide) { width: 100%; }
	.kf-eps .kf-item { display: grid; grid-template-columns: 36px minmax(0, 1fr); align-items: center; gap: 14px; padding: 10px; border-radius: 16px; transition: background var(--dur-fast) var(--ease-out); }
	.kf-eps .kf-list { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 8px 16px; }
	.kf-eps .kf-item:hover { background: var(--surface-1); }
	.kf-eps .kf-n { grid-column: 1; grid-row: 1; color: var(--text-muted); font: 800 22px/1 var(--font-display); font-variant-numeric: tabular-nums; text-align: center; }
	.kf-eps .kf-pic { grid-column: 2; grid-row: 1; }
	
	@media (prefers-reduced-motion: reduce) { .kf-item { animation: none; } }
	.c-line { display: flex; flex-wrap: wrap; align-items: baseline; }
	/* The About sheet is a dark window in the person's tint. */
	:global(.ui-sheet:has(.ab)) { --text: #fff; --text-muted: rgb(235 235 245 / .62); --surface-1: rgb(255 255 255 / 7%); --surface-2: rgb(255 255 255 / 12%); --line: rgb(255 255 255 / 10%); --border: rgb(255 255 255 / 10%); color-scheme: dark; color: #fff; background: radial-gradient(120% 70% at 50% 0%, color-mix(in srgb, var(--ap-bg, #2a2c33) 85%, #fff 6%), color-mix(in srgb, var(--ap-bg, #1b1c20) 55%, #050506) 70%); box-shadow: 0 0 0 1px rgb(255 255 255 / 8%), 0 40px 120px rgb(0 0 0 / .6); }
	.ab { display: flex; flex-direction: column; gap: 28px; }
	.ab-switch { display: flex; justify-content: center; }
	.ab-still { position: relative; margin: 0; aspect-ratio: 21 / 9; border-radius: 18px; box-shadow: 0 20px 50px rgb(0 0 0 / .45); overflow: hidden; background: #1c1d21; isolation: isolate; animation: ab-in var(--dur-slow) var(--ease-out); }
	.ab-frame { position: absolute; inset: 0; z-index: -1; width: 100%; height: 100%; object-fit: cover; object-position: center 25%; opacity: 0; scale: 1.02; transition: opacity 1.2s var(--ease-out), scale 7s linear; }
	.ab-frame.on { opacity: 1; scale: 1.12; }
	.ab-still::after { content: ''; position: absolute; inset: 0; z-index: -1; background: linear-gradient(to top, rgb(0 0 0 / .78), rgb(0 0 0 / 0) 60%); }
	.ab-still figcaption { position: absolute; left: 24px; right: 24px; bottom: 20px; display: flex; align-items: center; gap: 16px; color: #fff; }
	.ab-face { flex: none; width: 64px; height: 96px; border-radius: 10px; object-position: center 20%; object-fit: cover; box-shadow: 0 0 0 2px rgb(255 255 255 / .85), 0 8px 24px rgb(0 0 0 / .4); }
	.ab-still figcaption span { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
	.ab-still b { font: 800 clamp(24px, 3vw, 34px)/1 var(--font-display); letter-spacing: -.02em; }
	.ab-still small { font: 600 14px/1.3 var(--font-ui); opacity: .82; }
	/* Minimal: the face and the name as a plain row, no stills. */ .ab-still.plain { aspect-ratio: auto; border-radius: 0; box-shadow: none; overflow: visible; background: none; animation: none; } .ab-still.plain::after { content: none; } .ab-still.plain figcaption { position: static; color: var(--text); } .ab-still.plain .ab-face { box-shadow: 0 0 0 1px var(--line); } :global(html[data-look='minimal'] .ui-sheet:has(.ab)) { --text: inherit; --text-muted: inherit; --surface-1: inherit; --surface-2: inherit; --line: inherit; --border: inherit; color-scheme: inherit; color: var(--text); background: var(--surface-1); box-shadow: var(--shadow-modal); } :global(html[data-look='minimal']) .ab-tiles div { background: var(--surface-2); box-shadow: inset 0 0 0 1px var(--line); backdrop-filter: none; -webkit-backdrop-filter: none; } :global(html[data-look='minimal']) .ab-tiles div:hover { background: var(--surface-3, var(--surface-2)); } :global(html[data-look='minimal']) .ab-bio p, :global(html[data-look='minimal']) .ab-bio p.lead { color: var(--text); }
	.ab-tiles { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 10px; margin: 0; }
	.ab-tiles div { display: flex; flex-direction: column; gap: 6px; padding: 16px 18px; border-radius: 16px; background: rgb(255 255 255 / 7%); box-shadow: inset 0 0 0 1px rgb(255 255 255 / 8%), inset 0 1px 0 rgb(255 255 255 / 10%); backdrop-filter: blur(24px) saturate(160%); transition: background var(--dur-fast) var(--ease-out), translate var(--dur-fast) var(--ease-out); }
	.ab-tiles div:hover { background: rgb(255 255 255 / 11%); translate: 0 -1px; }
	.ab-tiles dt, .ab-dl dt { margin: 0; color: var(--text-muted); font: 600 12px/1 var(--font-ui); text-transform: uppercase; letter-spacing: .06em; }
	.ab-sec h3 { margin: 0; color: var(--text); font: 700 20px/1.2 var(--font-display); letter-spacing: -.01em; }
	.ab-tiles div { min-width: 0; }
	.ab-tiles dd { overflow-wrap: anywhere; text-wrap: balance; margin: 0; color: var(--text); font: 700 17px/1.25 var(--font-display); letter-spacing: -.01em; }
	.ab-tiles small, .ab-dl small { color: var(--text-muted); font: var(--t-footnote); }
	.ab-body { display: flex; flex-direction: column; gap: 28px; }
	.ab-dossier .ab-body { display: grid; grid-template-columns: 240px minmax(0, 1fr); gap: 36px; align-items: start; }
	.ab-side { position: sticky; top: 0; display: flex; flex-direction: column; align-items: center; gap: 14px; text-align: center; }
	.ab-round { width: 168px; height: 168px; border-radius: 50%; overflow: hidden; background: var(--surface-2); box-shadow: 0 14px 36px rgb(0 0 0 / .3); }
	.ab-round img { width: 100%; height: 100%; object-fit: cover; }
	.ab-name { margin: 0; font: 800 24px/1.1 var(--font-display); letter-spacing: -.02em; color: var(--text); }
	.ab-dl { display: flex; flex-direction: column; width: 100%; margin: 0; text-align: left; }
	.ab-dl div { display: grid; grid-template-columns: 96px minmax(0, 1fr); gap: 12px; padding: 12px 0; border-top: 1px solid var(--line, var(--border)); }
	.ab-dl dt { padding-top: 3px; }
	.ab-dl dd { display: flex; flex-direction: column; gap: 2px; margin: 0; color: var(--text); font: 600 15px/1.35 var(--font-ui); }
	.ab-main { display: flex; flex-direction: column; gap: 28px; min-width: 0; }
	.ab-sec { display: flex; flex-direction: column; gap: 14px; }
	.ab-bio { display: flex; flex-direction: column; gap: 14px; max-width: 68ch; }
	.ab-bio p { margin: 0; color: rgb(235 235 245 / .82); font: 400 16px/1.6 var(--font-ui); }
	.ab-bio p.lead { color: #fff; font: 500 19px/1.55 var(--font-ui); }
	.ab-bio p.lead::first-letter { float: left; margin: 4px 10px 0 0; font: 800 58px/.8 var(--font-display); color: var(--text); }
	.ab-awards { display: flex; flex-direction: column; margin: 0; padding: 0; list-style: none; }
	.ab-awards li { display: grid; grid-template-columns: 52px minmax(0, 1fr) auto; align-items: center; gap: 14px; padding: 12px 0; border-top: 1px solid var(--line, var(--border)); }
	.ab-yr { color: var(--text-muted); font: 600 14px/1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.ab-aw { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
	.ab-aw b { color: var(--text); font: 600 15px/1.3 var(--font-ui); }
	.ab-aw small { color: var(--text-muted); font: var(--t-footnote); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.ab-tag { padding: 4px 10px; border-radius: 999px; background: var(--surface-2); color: var(--text-muted); font: 700 12px/1 var(--font-ui); }
	.won .ab-tag { background: #f5c518; color: #111; }
	.ab-figs { display: grid; grid-template-columns: repeat(auto-fit, minmax(130px, 1fr)); gap: 0; border-block: 1px solid var(--line, var(--border)); }
	.ab-figs p { display: flex; flex-direction: column; align-items: center; gap: 6px; margin: 0; padding: 22px 8px; text-align: center; }
	.ab-figs p + p { border-left: 1px solid var(--line, var(--border)); }
	.ab-figs b { color: var(--text); font: 800 28px/1 var(--font-display); font-variant-numeric: tabular-nums; letter-spacing: -.02em; }
	.ab-figs span { color: var(--text-muted); font: var(--t-footnote); }
	.ab-bars { display: flex; align-items: flex-end; gap: 10px; height: 150px; margin: 0; padding: 0; list-style: none; }
	.ab-bars li { flex: 1; display: flex; flex-direction: column; align-items: center; gap: 6px; height: 100%; justify-content: flex-end; }
	.ab-bars .bar { display: flex; align-items: flex-end; width: 100%; max-width: 56px; height: 100%; }
	.ab-bars i { display: block; width: 100%; height: calc(var(--h) * 100%); min-height: 4px; border-radius: 8px 8px 3px 3px; background: linear-gradient(to top, color-mix(in srgb, var(--text) 30%, transparent), var(--text)); transform-origin: bottom; animation: ab-grow var(--dur-slow) var(--ease-out) both; }
	.ab-bars b { color: var(--text); font: 700 13px/1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.ab-bars small { color: var(--text-muted); font: 600 11px/1 var(--font-ui); }
	.ab-src { align-self: flex-start; color: var(--text-muted); font: 600 14px/1 var(--font-ui); text-decoration: underline; text-underline-offset: 3px; }
	@keyframes ab-in { from { opacity: 0; translate: 0 8px; } }
	@keyframes ab-grow { from { scale: 1 0; } }
	@media (prefers-reduced-motion: reduce) { .ab-still, .ab-bars i { animation: none; } .ab-frame { transition: opacity 1.2s; scale: 1; } }
	@media (max-width: 767px) {
			.kf-eps .kf-list { grid-template-columns: minmax(0, 1fr); }
		.kf-eps .kf-item { grid-template-columns: 28px minmax(0, 1fr); gap: 10px; padding: 6px; }
		.ab-dossier .ab-body { grid-template-columns: minmax(0, 1fr); gap: 24px; }
		.ab-side { position: static; }
		.ab-still { aspect-ratio: 16 / 10; }
		.ab-still figcaption { left: 16px; right: 16px; bottom: 14px; }
		.ab-face { width: 48px; height: 72px; }
		.ab-tiles { grid-template-columns: repeat(2, minmax(0, 1fr)); }
		.ab-figs { grid-template-columns: repeat(2, minmax(0, 1fr)); }
		.ab-figs p:nth-child(3) { border-left: 0; }
		.ab-figs p:nth-child(n + 3) { border-top: 1px solid var(--line, var(--border)); }
		.ab-bars { gap: 6px; height: 120px; }
		.ab-bio p.lead { font-size: 17px; }
		.ap { --gap: 32px; }
		.banner { height: 200px; margin-top: calc(var(--s-4) * -1); }
		.cline { grid-template-columns: minmax(0, 1fr); justify-items: center; text-align: center; gap: 12px; margin-top: calc(var(--gap) * -1 - 56px); }
		.photo { width: 112px; height: auto; }
		.c-words { align-items: center; padding: 0; }
		.c-words h1 { font-size: clamp(28px, 8.4vw, 36px); }
		.acts { padding: 0; }
		.slimwrap { top: 0; }
		:global(html.bar-hidden) .slimwrap { transform: none; }
		.slim { height: 56px; gap: 12px; }
		.slim-name { font-size: 18px; }
		.song { grid-template-columns: 24px 40px minmax(0, 1fr) auto; gap: 10px; padding: 0 4px; height: 72px; }
		.thumb { width: 40px; height: 60px; }
		.num { width: 24px; }
		.acard { height: 360px; }
		.disc { width: 76px; height: 76px; top: 14px; left: 14px; }
		.disc b { font-size: 21px; }
		.a-foot { left: 16px; right: 16px; bottom: 16px; }
		.a-bio { font-size: 17px; } .acard.minimal { height: auto; gap: 14px; padding: 16px; }
		.person { --title-item: calc((100cqi - var(--shelf-gap, 12px)) / 1.6 / .78); gap: 26px; }
		.hero-in { grid-template-columns: 96px minmax(0, 1fr); }
		.grid { grid-template-columns: repeat(auto-fill, minmax(min(100%, 260px), 1fr)); }
		.tiles { grid-template-columns: repeat(3, minmax(0, 1fr)); }
	}
</style>
