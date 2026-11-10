<script lang="ts">
	import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
	import '#lib/ui/title-page.css';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import TitleHero, { richMetadata, runtimeWords, type RichMeta } from '#lib/ui/TitleHero.svelte';
	import ActionButton from '#lib/ui/ActionButton.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import TitleRows from '#lib/ui/TitleRows.svelte';
	import FollowButton from '#lib/ui/FollowButton.svelte';
	import type { InfoDetails } from '#lib/ui/title-info.ts';
	import { tmdbArtName } from '#lib/ui/title-cards.ts';
	import { loadExtras, scoreName, type OutsideScore } from '#lib/ui/outside-ratings.ts';
	import { ratingPicks, shownRatings } from '#lib/ui/rating-picks.svelte.ts';
	import { loadWikiFacts, type WikiAward } from '#lib/ui/wiki-facts.ts';
	import ActionMenu from '#lib/ui/ActionMenu.svelte';
	import PersonCircle from '#lib/ui/PersonCircle.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import { loadAskedBy, type AskedByAnswer } from '#lib/ui/asked-by.ts';
	import AskedByLine from '#lib/ui/AskedByLine.svelte';
	import { audienceOf, titleStatus } from '#lib/ui/title-status.ts';
	import TitleScores from './TitleScores.svelte';
	import TitleTrailers from './TitleTrailers.svelte';
	import { createOwnedTitle } from './title-owned.svelte.ts';
	import TitleLibrary from './TitleLibrary.svelte';
	import TitleOwnedSheets from './TitleOwnedSheets.svelte';
	import { SearchOutcomeLine, QuickAsk, AddMovieDialog, AddShowDialog, AskShowDialog, AskMovieDialog } from '#lib/requests/index.ts';
	import { seasonWords, SeasonFolds } from '#lib/shows/index.ts';
	import { detailPath, fourKOffer, isDetailKind, loadDetails, remembered, type TitleDetails } from '#lib/metadata/index.ts';
	import type { GateDefaults } from '#lib/mediafiles/index.ts';
	import type { ShowSettings } from '#lib/shows/index.ts';
	import { savedDefaults } from '#lib/mediafiles/index.ts';
	import { goto, replaceState, snapshot } from '$app/navigation';
	import { page } from '$app/state';
	import { untrack } from 'svelte';
	import { to } from '#lib/core/paths.ts';
	import { api, message, ApiError } from '#lib/core/api.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { refreshInbox } from '#lib/core/inbox.svelte.ts';
	import { preloadHero } from '#lib/core/tmdb-image.ts';
	import type { Ask, LibraryFolder, LookupMovie, PickRulesView, TitleView } from '#lib/core/types.ts';
	import type { LookupShow } from '#lib/core/types.ts';
	import { bytes, ago } from '#lib/core/format.ts';
	import { live } from '#lib/core/live.svelte.ts';
	import { date } from '#lib/core/format.ts';

	// The one title page for a movie or show, wherever it is opened from (/title/<kind>/<tmdb> and /library/<id> both render it). Not in the Library: Add (people who manage it) or Ask (members). In the Library: the same page with its Library tab.
	let { kind, tmdb, libraryId, backHref }: { kind: string; tmdb: string; libraryId?: number; backHref?: string } = $props();
	let details = $state<TitleDetails | null>(null);
	// Back from a person (or any page opened here) lands where the page was. Rows below the hero load late, so the scroll is restored as they arrive.
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

	const own = createOwnedTitle();
	const manage = $derived(own.manage);
	const owned = $derived(own.owned);
	const ownState = $derived(own.ownState);
	const ownBadges = $derived(own.ownBadges);
	const ownTab = $derived(own.ownTab);
	const ownTabs = $derived(own.ownTabs);
	const ownedFailed = $derived(own.ownedFailed);
	const moreMenu = $derived(own.moreMenu);
	const mayReport = $derived(own.mayReport);
	// The short facts about the copy you have: quality, size, kept up to date.
	let rules = $state<PickRulesView[]>([]);
	let folders = $state<LibraryFolder[]>([]);
	// Who asked, for the hero byline.
	let askedBy = $state<AskedByAnswer>({ titles: {} });
	$effect(() => {
		const id = owned?.id;
		if (!id) return;
		loadAskedBy(api.get, [id]).then((x) => (askedBy = x), () => {});
	});
	const askers = $derived(owned ? (askedBy.titles[String(owned.id)]?.askers ?? []) : []);
	const mayAsk = $derived(session.can('ask') && !manage);
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
		// Opened from the Library: its record loads beside the details, not after them.
		if (libraryId && own.ownedFor !== libraryId) void own.loadOwned(libraryId);
		try {
			// The metadata answer draws the hero at once; the Library and ask facts follow.
			const d = await loadDetails(api.get, k, id, undefined, (m) => {
				if (k !== kind || id !== tmdb) return;
				preloadHero(m.backdrop);
				details = { ...m, titleId: m.titleId ?? details?.titleId };
			});
			if (k !== kind || id !== tmdb) return;
			preloadHero(d?.backdrop);
			if (!d) {
				missing = !details;
				return;
			}
			details = d;
			const ownId = d.titleId ?? libraryId;
			if (!ownId) {
				own.owned = null;
				own.ownedFor = 0;
			} else if (own.ownedFor !== ownId) {
				own.owned = null;
				void own.loadOwned(ownId);
			}
		} catch (e) {
			if (e instanceof ApiError && e.status === 403) {
				blocked = true;
				details = null;
			} else error = message(e);
		} finally {
			loading = false;
		}
	}
	// Load only when the title itself changes. A live update hands over the same kind and TMDB id, and reloading then would drop open sheets and scroll.
	let loadedKey = '';
	$effect(() => {
		const k = kind, id = tmdb;
		if (`${k}/${id}` === loadedKey) return;
		loadedKey = `${k}/${id}`;
		untrack(() => {
			void load(k, id);
			allCast = false;
		});
	});
	$effect(() => {
		if (!manage || loadedSettings) return;
		loadedSettings = true;
		api.get<{ library_folders: LibraryFolder[] }>('/library-folders').then((r) => (folders = r.library_folders)).catch(() => {});
		api.get<{ pick_rules: PickRulesView[] }>('/pick-rules').then((r) => (rules = r.pick_rules)).catch(() => {});
		api.get<GateDefaults>('/settings/release-gate').then((r) => (gateDefault = r.release_gate)).catch(() => {});
		api.get<ShowSettings>('/settings/shows').then((r) => (showDefaults = r)).catch(() => {});
		// The add dialogs wait for the saved defaults, fetched once per visit.
		savedDefaults.forget();
		void savedDefaults.load();
	});
	function added(title: TitleView) {
		toasts.push(t('discover.titlePage.added2', { name: title.name }), 'ok');
		void goto(to(`/library/${title.id}`));
	}
	function movieAsked(r: { ask: Ask; joined: boolean; already: boolean }) {
		const brief = { id: r.ask.id, state: r.ask.state, state_label: r.ask.state_label, mine: true, people: r.ask.people.length };
		if (details) details = r.ask.four_k ? { ...details, ask4k: brief } : { ...details, ask: brief };
		if (r.ask.four_k) toasts.push(t('discover.titlePage.requested4k', { name: r.ask.name }), 'ok', { body: r.joined ? t('discover.titlePage.onRequestToo') : t('discover.titlePage.waitingForApproval') });
		else if (r.ask.state === 'approved') toasts.push(t('discover.titlePage.approved', { name: r.ask.name }), 'ok', { body: t('discover.titlePage.searchingForRelease') });
		else if (r.joined) toasts.push(t('discover.titlePage.alreadyRequested', { name: r.ask.name }), 'ok', { body: t('discover.titlePage.onRequestToo') });
		else toasts.push(t('discover.titlePage.requested', { name: r.ask.name }), 'ok', { body: t('discover.titlePage.waitingForApproval') });
		void refreshInbox();
	}
	function showAsked(s: LookupShow, r: { ask: Ask; joined: boolean; already: boolean }) {
		if (details) details = { ...details, ask: { id: r.ask.id, state: r.ask.state, state_label: r.ask.state_label, mine: true, people: r.ask.people.length, seasons: r.ask.seasons } };
		const what = `${r.ask.name} (${seasonWords(r.ask.seasons)})`;
		if (r.ask.state === 'approved') toasts.push(t('discover.titlePage.approved', { name: what }), 'ok', { body: t('discover.titlePage.searchingForRelease') });
		else if (r.joined) toasts.push(t('discover.titlePage.alreadyRequested', { name: what }), 'ok', { body: t('discover.titlePage.onRequestToo') });
		else toasts.push(t('discover.titlePage.requested', { name: what }), 'ok', { body: t('discover.titlePage.waitingForApproval') });
		void refreshInbox();
	}
	const kindWord = $derived(kind === 'show' ? t('discover.titlePage.tvShows') : kind === 'movie' ? t('discover.titlePage.movies') : t('discover.titlePage.notFound'));
	// Other apps and old links say "tv" or "series" for a show and "film" for a movie; go to the one address this page uses.
	const KIND_ALIASES: Record<string, string> = { tv: 'show', series: 'show', shows: 'show', film: 'movie', movies: 'movie' };
	$effect(() => {
		const alias = KIND_ALIASES[kind.toLowerCase()];
		if (alias && typeof location !== 'undefined') void goto(location.pathname.replace(`/title/${kind}/`, `/title/${alias}/`) + location.search, { replaceState: true });
	});
	// The logo and age rating, when the metadata endpoint has them (cached, shared with loadDetails).
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
	// The hero says what it is (year, length or seasons, genres, age rating, scores); the details under the story say only what the hero does not.
	const personHref = (id?: string) => (id ? to(`/people/${id.replace(/^tmdb:/, '')}`) : undefined);
	/** The director (movies) or creators (shows). */
	const makers = $derived((info?.crew ?? []).filter((p) => p.role === (kind === 'show' ? 'creator' : 'director')));
	const madeBy = $derived(kind === 'show' ? t('discover.titlePage.createdBy') : t('discover.titlePage.directedBy'));
	const stars = $derived((details?.cast ?? []).slice(0, 3).map((p) => ({ name: p.name, id: p.id })));
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
	// The scores sit under the story, not in the hero.
	let outside = $state<OutsideScore[]>([]);
	$effect(() => {
		const k = kind, id = tmdb;
		outside = [];
		if (!isDetailKind(k) || !id) return;
		let live = true;
		void loadExtras(api.get, k, id).then((x) => live && (outside = x.scores));
		return () => (live = false);
	});
	// Only the ratings this browser picked (Settings > Appearance).
	const picked = $derived(shownRatings(outside, rich?.score ?? scoreOf(details?.rating), ratingPicks.list, !ratingPicks.custom));
	const hasScores = $derived(picked.scores.length > 0 || !!picked.tmdb);
	// The second row of facts: dates, where the title stands, studio, money, awards, original title.
	const fmtDay = (d?: string) => (d ? new Date(`${d}T00:00:00Z`).toLocaleDateString(appLocale(), { month: 'short', day: 'numeric', year: 'numeric', timeZone: 'UTC' }) : '');
	const money = (n?: number) => (n ? new Intl.NumberFormat(appLocale('en-US'), { style: 'currency', currency: 'USD', notation: 'compact', maximumFractionDigits: 1 }).format(n) : '');
	const statusWord = (s?: string) => (!s || s === 'Released' ? '' : s.charAt(0) + s.slice(1).toLowerCase());
	// Award badges ("3 Oscars"), in the hero or under it with the facts line.
	let awards = $state<WikiAward[]>([]);
	let awardBadges = $state<string[]>([]);
	$effect(() => {
		const k = kind, id = tmdb;
		awards = [];
		awardBadges = [];
		if (!isDetailKind(k) || !id) return;
		let live = true;
		void loadWikiFacts(api.get, k, id).then((f) => live && ((awards = f.awards), (awardBadges = f.badges.slice(0, 2))));
		return () => (live = false);
	});
	const topScores = $derived(picked.scores);
	const topTmdb = $derived(picked.tmdb);
	const tags = $derived((info?.keywords ?? []).slice(0, 10));
	const cert = $derived(details ? (rich?.certification ?? certOf(details.rating) ?? '') : '');
	// About band: every score this title has (first is the big one), the award count, and three fact columns.
	const allScores = $derived.by(() => {
		const out: { name: string; value: string; votes: string }[] = [];
		// Every score on one scale (out of 10, one decimal) so rows line up and compare at a glance.
		const ten = (v: number, max: number) => ((Math.round((v / max) * 100) / 10).toFixed(1));
		for (const o of outside) out.push({ name: scoreName(o.source), value: ten(o.value, o.max), votes: o.votes ? o.votes.toLocaleString(appLocale()) : '' });
		const t = rich?.score ?? scoreOf(details?.rating);
		if (t) out.push({ name: 'TMDB', value: ten(t, 10), votes: '' });
		return out;
	});
	const awardWins = $derived(awards.filter((a) => a.won).length);
	const atvCols = $derived.by(() => {
		if (!details) return [];
		const d = details;
		const i = info;
		const row = (label: string, value: string | number | undefined | null) => (value ? [{ label, value: String(value) }] : []);
		const ended = i?.status === 'Ended' || i?.status === 'Canceled';
		const eps = seasons.filter((x) => x.number > 0).reduce((n, x) => n + (x.episodes ?? 0), 0);
		const information = [
			...row(network ? t('discover.titlePage.network') : i?.studios.length ? t('discover.titlePage.studio') : '', network || i?.studios.slice(0, 2).join(', ')),
			...row(d.kind === 'show' ? t('discover.titlePage.aired') : t('discover.titlePage.released'), d.kind === 'show' ? (i?.dates.firstAir ? `${i.dates.firstAir.slice(0, 4)}–${ended && i.dates.lastAir ? i.dates.lastAir.slice(0, 4) : t('discover.titlePage.now')}` : d.year) : i?.dates.theaters ? fmtDay(i.dates.theaters) : d.year),
			...row(d.kind === 'show' ? t('discover.titlePage.seasons') : t('discover.titlePage.runTime'), d.kind === 'show' ? (regular ? t('discover.titlePage.seasonsEpisodes', { regular, eps }) : '') : runtimeWords(d.runtime)),
			...row(t('discover.titlePage.rated'), cert),
			...row(t('discover.titlePage.status'), d.kind === 'show' ? statusWord(i?.status) || i?.status : ''),
			...row(countries.length > 1 ? t('discover.titlePage.regionsOfOrigin') : t('discover.titlePage.regionOfOrigin'), countries.join(', '))
		].filter((r) => r.label);
		const languages = [
			...row(t('discover.titlePage.originalLanguage'), lang),
			...row(t('discover.titlePage.spokenLanguages'), (i?.spokenLanguages.length ?? 0) > 1 ? i?.spokenLanguages.join(', ') : ''),
			...row(t('discover.titlePage.originalTitle'), i?.originalTitle)
		];
		const release = [
			...row(t('discover.titlePage.digital'), i?.dates.digital ? fmtDay(i.dates.digital) : ''),
			...row(t('discover.titlePage.onDisc'), i?.dates.physical ? fmtDay(i.dates.physical) : ''),
			...row(t('discover.titlePage.nextEpisode'), i?.nextEpisode ? `${fmtDay(i.nextEpisode.airDate)} · S${i.nextEpisode.season} E${i.nextEpisode.episode}` : ''),
			...row(t('discover.titlePage.budget'), money(i?.budget)),
			...row(t('discover.titlePage.boxOffice'), money(i?.revenue))
		];
		return [
			{ title: t('discover.titlePage.information'), rows: information },
			{ title: t('discover.titlePage.languages'), rows: languages },
			{ title: d.kind === 'show' ? t('discover.titlePage.schedule') : t('discover.titlePage.releaseBoxOffice'), rows: release }
		].filter((c) => c.rows.length);
	});
	const outLinks = $derived.by(() => {
		if (!details) return [];
		const d = details;
		const ids = (d.kind === 'movie' ? d.movie?.ids : d.show?.ids) as Record<string, string | undefined> | undefined;
		return [
			ids?.imdb ? { name: 'IMDb', href: `https://www.imdb.com/title/${ids.imdb}/` } : null,
			{ name: 'TMDB', href: `https://www.themoviedb.org/${d.kind === 'show' ? 'tv' : 'movie'}/${d.tmdb}` },
			{ name: 'Trakt', href: `https://trakt.tv/search/tmdb/${d.tmdb}?id_type=${d.kind}` },
			d.kind === 'movie' ? { name: 'Letterboxd', href: `https://letterboxd.com/tmdb/${d.tmdb}/` } : null
		].filter((x): x is { name: string; href: string } => !!x);
	});
	const crumbs = $derived(details ? [{ href: to('/explore'), label: t('discover.titlePage.explore') }, { href: to(detailPath(details.kind, details.tmdb)), label: details.year ? `${details.name} (${details.year})` : details.name }] : []);
	// The breadcrumb's parent for a page opened fresh is the title's section (Movies or TV shows). From the Library grid it goes back to the grid with its filters (backHref).
	const sectionCrumb = $derived(details?.kind === 'show' ? { label: t('discover.titlePage.tvShows'), href: backHref ?? to('/library?kind=show') } : { label: t('discover.titlePage.movies'), href: backHref ?? to('/library?kind=movie') });
</script>

<svelte:head><title>{details?.name ? `${details.name} · HolyArrs` : `${kindWord} · HolyArrs`}</title></svelte:head>

{#if blocked}
	<EmptyState icon="shield" title={t('discover.titlePage.thisTitleIsNot')} action={{ label: t('discover.titlePage.backToExplore'), href: to('/explore') }} />
{:else if missing}
	<EmptyState icon="search" title={t('discover.titlePage.weCouldNotFind')} text={t('discover.titlePage.itMayHaveBeen')} action={{ label: t('discover.titlePage.search'), href: to('/search') }} />
{:else if error && !details}
	<LoadError hasData={false} message={t('discover.titlePage.couldNotLoadThis', { error })} onretry={() => void load(kind, tmdb)} />
{:else}
<PageLoader loading={!details || !details.name} label={t('discover.titlePage.loadingTheTitle')} onretry={() => void load(kind, tmdb)}>
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
			breadcrumb={isDetailKind(kind) ? [sectionCrumb, ...crumbs.slice(1)] : []}
			meta={heroFacts(d)}
			showFacts={false}
			ratings={false}
			tagline={info?.tagline}
			certification={rich?.certification ?? certOf(d.rating)}
			score={rich?.score ?? scoreOf(d.rating)}
			status={d.neverAdd || d.ask ? heroPill(d).kind : undefined}
			statusLabel={d.neverAdd || d.ask ? heroPill(d).label : undefined}
			vtName={tmdbArtName(d.kind, d.tmdb)}
		>
			{#snippet byline()}{#if owned}<AskedByLine {askers} />{#if ownState.kind !== 'have' && ownState.kind !== 'downloading'}<SearchOutcomeLine titleId={owned.id} />{/if}{/if}{/snippet}
			{#snippet actions()}
				{#if owned}
					<ActionButton icon={ownState.kind === 'have' ? 'check' : ownState.kind === 'failed' ? 'refresh' : ownState.kind === 'downloading' ? 'download' : 'search'} label={ownState.label} caption={ownState.kind === 'failed' && manage ? t('discover.titlePage.retry') : ownState.kind === 'missing' && manage ? t('discover.titlePage.searchNow') : ownState.label} primary onclick={() => (manage && (ownState.kind === 'failed' || ownState.kind === 'missing') ? own.ownRun('search') : own.setTab('library'))} data-testid="hero-library" />
					{#if manage || mayReport}<ActionMenu label={t('discover.titlePage.moreActions')} caption={t('discover.titlePage.more')} icon="more" items={moreMenu} />{/if}
					{#if mayAsk}{@render ask4kButton(d)}{/if}
				{:else if manage && !d.neverAdd}
					{#if d.kind === 'movie' && d.movie}
						<ActionButton icon="plus" label={t('discover.titlePage.addToTheLibrary', { name: d.name })} caption={t('discover.titlePage.addToLibrary')} primary onclick={() => addMovie?.open(d.movie as LookupMovie)} />
					{:else if d.kind === 'show' && d.show}
						<ActionButton icon="plus" label={t('discover.titlePage.addToTheLibrary', { name: d.name })} caption={t('discover.titlePage.addToLibrary')} primary onclick={() => addShow?.open(d.show as LookupShow)} />
					{/if}
				{:else if mayAsk && !d.neverAdd}
					{#if d.kind === 'movie'}
						<ActionButton icon="wish" label={d.ask ? t('discover.titlePage.askForToo', { name: d.name }) : t('discover.titlePage.askFor', { name: d.name })} caption={d.ask?.mine ? t('discover.titlePage.requestedCaption') : d.ask ? t('discover.titlePage.requestToo') : t('discover.titlePage.request')} primary disabled={!!d.ask?.mine} onclick={() => askMovieDialog?.open({ tmdb: d.tmdb, name: d.name, year: d.year, ask: d.ask, ask4k: d.ask4k })} />
						{@render ask4kButton(d)}
					{:else if d.show}
						<ActionButton icon="wish" label={d.ask ? t('discover.titlePage.askForToo', { name: d.name }) : t('discover.titlePage.askFor', { name: d.name })} caption={d.ask ? t('discover.titlePage.requestToo') : t('discover.titlePage.request')} primary onclick={() => askShow?.open(d.show as LookupShow)} />
					{/if}
				{/if}
				{#if d.kind === 'show' || d.kind === 'movie'}<FollowButton titleId={owned?.id} tmdb={d.tmdb} kind={d.kind} />{/if}
			{/snippet}
		</TitleHero>

		{#snippet ask4kButton(td: TitleDetails)}
			{@const offer = fourKOffer({ ...td, titleId: td.titleId ?? owned?.id })}
			{#if offer}
				<ActionButton icon="wish" pill label={offer === 'asked' ? t('discover.titlePage.askedFor4k', { name: td.name }) : t('discover.titlePage.askFor4k', { name: td.name })} caption={offer === 'asked' ? t('discover.titlePage.requested4kCaption') : t('discover.titlePage.request4k')} disabled={offer === 'asked'} onclick={() => askMovieDialog?.open({ tmdb: td.tmdb, name: td.name, year: td.year, ask4k: td.ask4k, fourKOnly: true })} data-testid="hero-ask-4k" />
			{/if}
		{/snippet}
		{#snippet castRow()}
			{#if d.cast.length}
				<Shelf title={t('discover.titlePage.castAndCrew')} item="112px" anchor="title-cast" whole onseeall={() => (allCast = true)}>
					{#each d.cast.slice(0, 12) as p, i (p.name + i)}
						<PersonCircle name={p.name} role={p.role} image={p.image} href={personHref(p.id)} />
					{/each}
				</Shelf>
			{/if}
		{/snippet}
		{#snippet names(list: { name: string; id?: string }[], max: number)}
			{#each list.slice(0, max) as p, i (p.name + i)}{#if i}{', '}{/if}{#if personHref(p.id)}<a href={personHref(p.id)}>{p.name}</a>{:else}{p.name}{/if}{/each}
		{/snippet}
		{#snippet story()}
			{#if d.overview}<p class="lead" data-testid="title-story">{d.overview}</p>{/if}
		{/snippet}
		{#snippet trailers()}
			<TitleTrailers kind={d.kind} tmdb={d.tmdb} title={d.name} videos={rich?.videos ?? []} art={[d.backdrop, ...(rich?.backdrops ?? [])]} />
		{/snippet}

		{#snippet factsLine()}
			{@const parts = heroFacts(d).slice(0, heroFacts(d).length - d.genres.slice(0, d.kind === 'show' ? 2 : 3).length)}
			<div class="calm-meta">
				{#each parts as f, i (i)}{#if i}<span class="sep" aria-hidden="true">·</span>{/if}<span>{f}</span>{/each}
				{#if cert}<span class="sep" aria-hidden="true">·</span><span class="fb cert">{cert}</span>{/if}
				{#if topScores.length || topTmdb}<span class="sep" aria-hidden="true">·</span><a class="top-scores" href="#ratings" title={t('discover.titlePage.allRatings')}><TitleScores scores={topScores} tmdb={topTmdb} variant="mono" /></a>{/if}
			</div>
		{/snippet}
		{#snippet tagLine()}
			{#if tags.length}<ul class="tagchips quiet" data-testid="title-tags">{#each tags.slice(0, 5) as t (t)}<li>{t}</li>{/each}</ul>{/if}
		{/snippet}
		{#snippet ownTabBar()}
			<div class="own-tabs" role="tablist" data-testid="own-tabs">
				{#each ownTabs as tab (tab.v)}
					<button type="button" role="tab" aria-selected={ownTab === tab.v} tabindex={ownTab === tab.v ? 0 : -1} class:on={ownTab === tab.v} onclick={() => own.setTab(tab.v)} onkeydown={(e) => { const d = e.key === 'ArrowRight' ? 1 : e.key === 'ArrowLeft' ? -1 : 0; if (!d) return; e.preventDefault(); const el = e.currentTarget as HTMLElement; const i = ownTabs.findIndex((x) => x.v === ownTab); const next = ownTabs[(i + d + ownTabs.length) % ownTabs.length]; own.setTab(next.v); requestAnimationFrame(() => el.parentElement?.querySelector<HTMLElement>('[aria-selected="true"]')?.focus()); }}>
						{tab.label}{#if tab.count}<span class="own-count">{tab.count}</span>{/if}{#if tab.alert}<span class="own-badge" aria-hidden="true">!</span><span class="sr-only">{t('discover.titlePage.needsAttention')}</span>{/if}
					</button>
				{/each}
			</div>
		{/snippet}
		{#if !owned && ownedFailed && libraryId}
			<LoadError hasData={false} message={t('discover.titlePage.couldNotLoadThis2')} onretry={() => libraryId && void own.loadOwned(libraryId)} />
		{/if}
		{#if owned}
			{@render ownTabBar()}
			<TitleLibrary {own} {folders} {rules} />
			<TitleOwnedSheets {own} />
		{/if}
		{#if !(owned && ownTab !== 'overview')}
		<div class="side" data-testid="title-side">
			<div class="d-left">{@render factsLine()}{@render story()}{@render tagLine()}</div>
			<dl class="side-credits">
				{#if makers.length}<div><dt>{madeBy}</dt><dd>{@render names(makers, 2)}</dd></div>{/if}
				{#if stars.length}<div><dt>{t('discover.titlePage.starring')}</dt><dd>{@render names(stars, 3)}</dd></div>{/if}
				{#if d.genres.length}<div><dt>{t('discover.titlePage.genres')}</dt><dd>{d.genres.slice(0, 3).join(', ')}</dd></div>{/if}
				{#if network}<div><dt>{t('discover.titlePage.network')}</dt><dd>{network}</dd></div>
				{:else if info?.studios.length}<div><dt>{t('discover.titlePage.studio')}</dt><dd>{info.studios[0]}</dd></div>{/if}
			</dl>
		</div>
		{@render trailers()}
		{#if d.kind === 'show'}<SeasonFolds tmdb={d.tmdb} {seasons} fallback={d.backdrop} />{/if}
		{@render castRow()}
		<TitleRows kind={d.kind} tmdb={d.tmdb} bind:info band={false} quickAsk={QuickAsk} />
			<!-- About band: a lighter strip to the page edges, a row of cards (the title, every score, awards), three columns of label and value facts, then links out. -->
			<section class="atv" data-testid="title-ledger">
				<h2 class="atv-h">{t('discover.titlePage.about')}</h2>
				<div class="atv-cards">
					{#if owned}
						<article class="atv-card" data-testid="own-file-card">
							<h3>{ownState.label}</h3>
							<dl class="atv-rows">
								{#if ownBadges.length}<div><dt>{t('discover.titlePage.quality')}</dt><dd>{ownBadges.join(' · ')}</dd></div>{/if}
								{#if owned.size_bytes}<div><dt>{t('discover.titlePage.size')}</dt><dd>{bytes(owned.size_bytes)}</dd></div>{/if}
								{#if owned.added_at}<div><dt>{t('discover.titlePage.added')}</dt><dd>{ago(owned.added_at)}</dd></div>{/if}
								<div><dt>{t('discover.titlePage.keptUpToDate')}</dt><dd>{owned.keep_up_to_date ? t('discover.titlePage.yes') : t('discover.titlePage.no')}</dd></div>
							</dl>
						</article>
					{:else}
					<article class="atv-card">
						<h3>{d.name}</h3>
						<p class="atv-cap">{d.genres.slice(0, 2).join(', ') || (d.kind === 'show' ? t('discover.titlePage.tvShow') : t('discover.titlePage.movie'))}</p>
						{#if d.overview}<p class="atv-text clamp">{d.overview}</p>{/if}
					</article>
					{/if}
					{#if allScores.length}
						<article class="atv-card">
							<h3 id="ratings">{t('discover.titlePage.ratings')}</h3>
							<div class="atv-scores" ><TitleScores scores={outside} tmdb={rich?.score ?? scoreOf(d.rating)} variant="list" outOf10 /></div>
						</article>
					{/if}
					{#if awards.length}
						<article class="atv-card">
							<h3>{awardBadges[0] ?? t('discover.titlePage.winsCount', { n: awardWins })}</h3>
								<dl class="atv-rows">
									<div><dt>{t('discover.titlePage.wins')}</dt><dd>{awardWins}</dd></div>
									<div><dt>{t('discover.titlePage.nominations')}</dt><dd>{awards.length - awardWins}</dd></div>
								</dl>
						</article>
					{/if}
				</div>
				<h3 class="atv-h3">{t('discover.titlePage.information')}</h3>
				<dl class="atv-grid">{#each atvCols.flatMap((c) => c.rows) as r (r.label)}<div><dt>{r.label}</dt><dd>{r.value}</dd></div>{/each}</dl>
				<p class="atv-links">{#each outLinks as l, i (l.name)}{#if i}<span class="sep" aria-hidden="true">·</span>{/if}<a href={l.href} target="_blank" rel="noopener noreferrer">{l.name} ↗</a>{/each}</p>
			</section>

		{/if}
		<Sheet bind:open={allCast} title={t('discover.titlePage.castAndCrew')} size="lg">
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

{#if manage}
	<AddMovieDialog bind:this={addMovie} {folders} {rules} {gateDefault} onadded={(_m, t) => added(t)} />
	<AddShowDialog bind:this={addShow} {folders} {rules} defaults={showDefaults} onadded={(_s, t) => added(t)} />
{/if}
{#if mayAsk}
	<AskMovieDialog bind:this={askMovieDialog} onasked={(_m, r) => movieAsked(r)} onrefused={(_m, body) => { if (typeof body.title_id === 'number') { toasts.push(t('discover.titlePage.alreadyInLibrary', { name: details?.name ?? kindWord }), 'info'); void goto(to(`/library/${body.title_id}`)); } }} />
	<AskShowDialog bind:this={askShow} onasked={showAsked} onrefused={(_s, body) => { if (typeof body.title_id === 'number') { toasts.push(t('discover.titlePage.alreadyInLibrary', { name: details?.name ?? kindWord }), 'info'); void goto(to(`/library/${body.title_id}`)); } }} />
{/if}

<style>
	.all-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(132px, 1fr)); gap: 18px 14px; margin: 0; padding: 0; list-style: none; }
	.all-grid.people { grid-template-columns: repeat(auto-fill, minmax(104px, 1fr)); }
	.lead { max-width: 72ch; margin: 0; font: 400 16px/1.55 var(--font-ui); color: var(--text); }
	.d-left { display: flex; flex-direction: column; gap: 18px; min-width: 0; }
	.top-scores { display: contents; color: inherit; text-decoration: none; }
	#ratings { scroll-margin-top: 90px; }
	.own-tabs { display: flex; gap: 6px; }
	.own-tabs button { position: relative; display: inline-flex; align-items: center; gap: 6px; height: 40px; padding: 0 14px; border: 0; background: none; color: var(--text-muted); font: 600 14px/1 var(--font-ui); cursor: pointer; }
	.own-tabs button.on { color: var(--text); }
	.own-count { color: var(--text-muted); font: 600 12px/1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.own-tabs { border-bottom: 1px solid var(--line); }
	.own-tabs button { border-bottom: 2px solid transparent; transition: color var(--dur-fast) var(--ease-out), border-color var(--dur-fast) var(--ease-out); }
	.own-tabs button:hover { color: var(--text); }
	.own-tabs button.on { border-bottom-color: var(--text); }
	@keyframes own-pulse { 50% { opacity: .35; } }
	
	.own-badge { margin-left: 6px; display: inline-grid; place-items: center; width: 16px; height: 16px; border-radius: 50%; background: var(--fail); color: #fff; font-size: 11px; }
	.calm-meta .sep { margin: 0 8px; color: var(--text-faint); }
	.side-credits dt { color: var(--text-muted); font-weight: 500; }
	.side-credits a { color: inherit; }
	.side { display: grid; grid-template-columns: minmax(0, 1.6fr) minmax(0, 1fr); gap: 20px 48px; align-items: start; padding-bottom: 8px; }
	.side-credits { display: grid; gap: 10px; margin: 0; font: 500 14px/1.45 var(--font-ui); }
	.side-credits div { display: flex; flex-wrap: wrap; gap: 0 6px; }
	.side-credits dd { margin: 0; color: var(--text); }
	@media (max-width: 899px) { .side { grid-template-columns: minmax(0, 1fr); } }
	.calm-meta { margin: 0; display: flex; flex-wrap: wrap; align-items: center; gap: 6px 0; color: var(--text-muted); font: 500 15.5px/1.4 var(--font-ui); }
	.d-left .tagchips.quiet { margin-top: -6px; }
	.atv { position: relative; margin-top: 32px; padding: 32px 0 40px; background: var(--atv-band); box-shadow: 0 0 0 100vmax var(--atv-band); clip-path: inset(0 -100vmax -100vmax -100vmax); --atv-band: color-mix(in srgb, var(--text) 7%, var(--bg)); --atv-card: rgb(255 255 255 / 10%); }
	:global([data-theme='light']) .atv { --atv-band: color-mix(in srgb, var(--text) 5%, var(--bg)); --atv-card: var(--bg); }
	.atv-scores :global(.list li) { padding: 0; border-top: 0; font-size: 15px; line-height: 20px; }
	.atv-scores :global(.list .n) { color: var(--text-muted); font: 400 15px/20px var(--font-ui); }
	.atv-scores :global(.list .v) { color: var(--text-muted); font: 400 15px/20px var(--font-ui); font-variant-numeric: tabular-nums; }
	.atv-h { margin: 0 0 13px; color: var(--text); font: 700 17px/21px var(--font-ui); }
	.atv-cards { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 20px; }
	.atv-card { min-width: 0; padding: 14px 16px 16px; border-radius: 12px; background: var(--atv-card); }
	.atv-card h3 { margin: 0; padding-bottom: 8px; color: var(--text); font: 700 17px/21px var(--font-ui); }
	.atv-cap { margin: 0; padding-bottom: 5px; color: var(--text-muted); font: 400 11px/14px var(--font-ui); text-transform: uppercase; }
	.atv-text { margin: 0; color: var(--text-muted); font: 400 15px/20px var(--font-ui); }
	.atv-text.clamp { display: -webkit-box; -webkit-line-clamp: 4; line-clamp: 4; -webkit-box-orient: vertical; overflow: hidden; }
	.atv-rows { display: grid; gap: 2px; margin: 0; }
	.atv-rows div { display: flex; justify-content: space-between; gap: 12px; color: var(--text-muted); font: 400 15px/20px var(--font-ui); }
	.atv-rows dd { margin: 0; color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.atv-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 20px; margin: 0; }
	.atv-grid dt { color: var(--text); font: 400 12px/16px var(--font-ui); }
	.atv-grid dd { margin: 0; color: var(--text-muted); font: 400 12px/16px var(--font-ui); }
	.atv-h3 { margin: 22px 0 13px; color: var(--text); font: 700 17px/21px var(--font-ui); }
	.atv-links { display: flex; flex-wrap: wrap; align-items: center; margin: 20px 0 0; color: var(--text-muted); font: 400 12px/16px var(--font-ui); }
	.atv-links .sep { margin: 0 8px; color: var(--text-faint); }
	.atv-links a { color: var(--text-muted); text-decoration: none; }
	/* A 44px-tall hit area over the 16px link line on touch phones. */
	@media (max-width: 767px) and (pointer: coarse) { .atv-links a { position: relative; } .atv-links a::after { content: ''; position: absolute; inset: -14px 0; } }
	.atv-links a:hover { color: var(--text); }
	@media (max-width: 899px) { .atv-cards { grid-template-columns: minmax(0, 1fr); } .atv-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px 16px; } }
	.tagchips.quiet li { background: none; border: 1px solid var(--line); color: var(--text-muted); font-size: 12px; }
	.fb { display: inline-flex; align-items: center; gap: 4px; height: 22px; padding: 0 7px; border-radius: 5px; border: 1.5px solid currentColor; font: 700 11.5px/1 var(--font-ui); letter-spacing: .03em; white-space: nowrap; }
	.tagchips { display: flex; flex-wrap: wrap; gap: 6px; margin: 0; padding: 0; list-style: none; }
	.tagchips li { display: inline-flex; align-items: center; min-height: 30px; padding: 0 12px; border: 1px solid var(--line); border-radius: 999px; color: var(--text); font: 500 13px/1 var(--font-ui); }
</style>
