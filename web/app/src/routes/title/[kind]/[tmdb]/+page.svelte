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
	import { tmdbArtName } from '#lib/explore.ts';
	import SeasonFolds from '#lib/SeasonFolds.svelte';
	import TitleScores from '#lib/TitleScores.svelte';
	import TitleTrailers from '#lib/TitleTrailers.svelte';
	import { loadExtras, scoreName, scoreText, type OutsideScore } from '#lib/outside-ratings.ts';
	import { ratingPicks, shownRatings } from '#lib/rating-picks.svelte.ts';
	import { aboutPicks, aboutShown } from '#lib/about-picks.svelte.ts';
	import { awardsTip, loadWikiFacts, type WikiAward } from '#lib/wiki-facts.ts';
	import type { TagsAt, TitleLook } from '#lib/title-look.svelte.ts';
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
	// The hero says what it is (year, length or seasons, genres, age rating,
	// scores); the details under the story say only what the hero does not.
	const personHref = (id?: string) => (id ? to(`/people/${id.replace(/^tmdb:/, '')}`) : undefined);
	/** The director (movies) or creators (shows). */
	const makers = $derived((info?.crew ?? []).filter((p) => p.role === (kind === 'show' ? 'creator' : 'director')));
	const madeBy = $derived(kind === 'show' ? 'Created by' : 'Directed by');
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
	// Only the ratings this browser picked (Settings › Appearance).
	const picked = $derived(shownRatings(outside, rich?.score ?? scoreOf(details?.rating), ratingPicks.list, !ratingPicks.custom));
	const hasScores = $derived(picked.scores.length > 0 || !!picked.tmdb);
	// The second row of facts: dates, where the title stands, studio, money, awards, original title.
	const fmtMonth = (d: string) => new Date(`${d}T00:00:00Z`).toLocaleDateString(undefined, { month: 'short', year: 'numeric', timeZone: 'UTC' });
	const fmtDay = (d?: string) => (d ? new Date(`${d}T00:00:00Z`).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric', timeZone: 'UTC' }) : '');
	const money = (n?: number) => (n ? new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD', notation: 'compact', maximumFractionDigits: 1 }).format(n) : '');
	const statusWord = (s?: string) => (!s || s === 'Released' ? '' : s.charAt(0) + s.slice(1).toLowerCase());
	// Award badges ("3 Oscars"): in the hero, or under it with the facts line (H2).
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
	// Where the facts line and tagline go while the options are compared (title-look.svelte.ts).
	// The chosen layout (V5: facts row and story left, credits right; tags under the story).
	// The chosen layout (E): facts row and story left, credits right; Information at the bottom. Desktop first.
	// About cards: two wide ones (ratings, awards); no title card, the story is already at the top.
	// Whole-page views while Luis compares (?view=): Now, Before (the page before the reviewers' fixes) and three mixes.
	type View = { value: string; label: string; newWords: boolean; status: boolean; names: boolean; cards: 'two' | 'three'; missing: boolean; wide: boolean; heroFacts: boolean; band: 'side' | 'apple' | 'netflix' | 'compact' };
	const VIEWS: View[] = [
		{ value: 'now', label: 'Now', newWords: true, status: true, names: true, cards: 'three', missing: false, wide: true, heroFacts: false, band: 'side' },
		{ value: 'before', label: 'Before', newWords: false, status: false, names: false, cards: 'three', missing: true, wide: false, heroFacts: false, band: 'side' },
		{ value: 'm1', label: 'Mix 1 · Apple TV', newWords: true, status: true, names: true, cards: 'three', missing: false, wide: true, heroFacts: true, band: 'apple' },
		{ value: 'm2', label: 'Mix 2 · Netflix', newWords: true, status: true, names: true, cards: 'three', missing: false, wide: true, heroFacts: false, band: 'netflix' },
		{ value: 'm3', label: 'Mix 3 · One column', newWords: true, status: true, names: true, cards: 'three', missing: false, wide: true, heroFacts: false, band: 'compact' }
	];
	const viewName: string = 'now';
	const view = $derived(VIEWS.find((v) => v.value === viewName) ?? VIEWS[0]);
	const cardLook = $derived(view.cards);
	// The band under the hero: two columns (side), or one calm column (apple / compact, as the 'ledger' layout), or Netflix's split.
	const look = $derived<TitleLook>(view.band === 'side' ? 'side' : view.band === 'netflix' ? 'netflix1' : 'ledger');
	// The facts row shows the 3 picked scores (Settings › Appearance); the Ratings card at the bottom lists every source.
	const top: string = 'r1';
	const topScores = $derived(picked.scores);
	const topTmdb = $derived(picked.tmdb);
	const moreFacts = $derived.by((): [string, string][] => {
		if (!info) return [];
		const i = info;
		const studio = i.studios.filter((x) => x !== network);
		const rows: [string, string][] =
			kind === 'show'
				? [
						['Status', statusWord(i.status)],
						['Next episode', i.nextEpisode ? `${fmtDay(i.nextEpisode.airDate)} · S${i.nextEpisode.season} E${i.nextEpisode.episode}` : ''],
						['First aired', fmtDay(i.dates.firstAir)],
						['Last aired', i.status === 'Ended' || i.status === 'Canceled' ? fmtDay(i.dates.lastAir) : ''],
						[studio.length > 1 ? 'Studios' : 'Studio', studio.join(', ')]
					]
				: [
						['Status', statusWord(i.status)],
						['In theaters', fmtDay(i.dates.theaters)],
						['Digital', fmtDay(i.dates.digital)],
						['On disc', fmtDay(i.dates.physical)],
						[studio.length > 1 ? 'Studios' : 'Studio', studio.join(', ')],
						['Budget', money(i.budget)],
						['Box office', money(i.revenue)]
					];
		rows.push(['Original title', i.originalTitle ?? '']);
		return rows.filter(([, v]) => v);
	});
	const tags = $derived((info?.keywords ?? []).slice(0, 10));
	const cert = $derived(details ? (rich?.certification ?? certOf(details.rating) ?? '') : '');
	const lengthWord = $derived(details ? (details.kind === 'show' ? (regular ? `${regular} ${regular === 1 ? 'season' : 'seasons'}` : '') : runtimeWords(details.runtime)) : '');
 	// Every detail in one list, in reading order; the grid lays them out in even columns.
	type Fact = { label: string; kind: 'text' | 'makers' | 'cert' | 'awards' | 'link'; value: string };
	const facts = $derived.by((): Fact[] => {
		if (!details) return [];
		const d = details;
		const out: Fact[] = [];
		const t = (label: string, value: string | number | undefined | null) => value && out.push({ label, kind: 'text', value: String(value) });
		if (look === 'band') {
			t('Year', d.year);
			t(d.kind === 'show' ? 'Seasons' : 'Length', lengthWord);
			t(d.genres.length > 1 ? 'Genres' : 'Genre', d.genres.join(', '));
			if (cert) out.push({ label: 'Rated', kind: 'cert', value: cert });
			if (awardBadges.length) out.push({ label: 'Awards', kind: 'awards', value: awardBadges.join(', ') });
		}
		t('Network', network);
		if (makers.length) out.push({ label: madeBy, kind: 'makers', value: makers.map((m) => m.name).join(', ') });
		t('Language', lang);
		t(countries.length > 1 ? 'Countries' : 'Country', countries.join(', '));
		for (const [k, v] of moreFacts) t(k, v);
		return out;
	});
	// About, grouped in columns: Information, Made by, Language (and the tags when they live here).
	const aboutGroups = $derived.by(() => {
		if (!details) return [];
		const d = details;
		const pick = (labels: string[]) => facts.filter((f) => labels.includes(f.label) && f.kind !== 'cert' && f.kind !== 'awards');
		const info1: Fact[] = [];
		if (d.year) info1.push({ label: d.kind === 'show' ? 'Premiered' : 'Released', kind: 'text', value: String(d.year) });
		if (lengthWord) info1.push({ label: d.kind === 'show' ? 'Seasons' : 'Run time', kind: 'text', value: lengthWord });
		if (d.kind === 'show') {
			// One size line for whoever approves: 73 episodes · ~55 min each · ~67 h in all.
			const eps = seasons.filter((x) => x.number > 0).reduce((n, x) => n + (x.episodes ?? 0), 0);
			const per = info?.episodeRuntime ?? 0;
			if (eps) info1.push({ label: 'Episodes', kind: 'text', value: [`${eps}`, per ? `~${runtimeWords(per)} each` : '', per ? `~${Math.round((eps * per) / 60)} h in all` : ''].filter(Boolean).join(' · ') });
			if (info?.showType) info1.push({ label: 'Type', kind: 'text', value: info.showType });
		}
		if (cert) info1.push({ label: 'Rated', kind: 'text', value: cert });
		// Status carries when an ended show stopped (Ended · May 2019).
		const ended = info?.status === 'Ended' || info?.status === 'Canceled';
		info1.push(...pick(['Status']).map((f) => (ended && info?.dates.lastAir ? { ...f, value: `${f.value} · ${fmtMonth(info.dates.lastAir)}` } : f)));
		info1.push(...pick(['Next episode', 'In theaters', 'Digital', 'On disc']));
		// No digital date yet for a movie still in (or coming to) theaters: say so.
		const th = info?.dates.theaters;
		if (d.kind === 'movie' && th && !info?.dates.digital && Date.parse(th) > Date.now() - 120 * 864e5) info1.push({ label: 'Digital', kind: 'text', value: `Not yet · in theaters ${fmtDay(th)}` });
		info1.push(...pick(['Country', 'Countries']).map((f) => ({ ...f, label: f.label === 'Countries' ? 'Regions of origin' : 'Region of origin' })));
		const spoken = info?.spokenLanguages ?? [];
		if (spoken.length > 1) info1.push({ label: 'Spoken languages', kind: 'text', value: spoken.join(', ') });
		else info1.push(...pick(['Language']).map((f) => ({ ...f, label: 'Original language' })));
		info1.push(...pick(['Original title']));
		// Who made it is in the credits beside the story (director or creators, network or studio); here the rest.
		const made = pick(d.kind === 'show' ? ['Studio', 'Studios', 'Budget', 'Box office'] : ['Budget', 'Box office']).map((f) => (f.label === 'Studios' ? { ...f, value: f.value.split(', ').slice(0, 3).join(', ') } : f));
		// Links out, for reading more before asking.
		const ids = (d.kind === 'movie' ? d.movie?.ids : d.show?.ids) as Record<string, string | undefined> | undefined;
		const links: Fact[] = [];
		if (ids?.imdb) links.push({ label: 'link:IMDb', kind: 'link', value: `https://www.imdb.com/title/${ids.imdb}/` });
		links.push({ label: 'link:TMDB', kind: 'link', value: `https://www.themoviedb.org/${d.kind === 'show' ? 'tv' : 'movie'}/${d.tmdb}` });
		links.push({ label: 'link:Trakt', kind: 'link', value: `https://trakt.tv/search/tmdb/${d.tmdb}?id_type=${d.kind}` });
		if (d.kind === 'movie') links.push({ label: 'link:Letterboxd', kind: 'link', value: `https://letterboxd.com/tmdb/${d.tmdb}/` });
		// Reception: every score this title has (not only the picked tiles), then its awards.
		const t = (label: string, value: string): Fact => ({ label, kind: 'text', value });
		const reception: Fact[] = [];
		const tmdbScore = rich?.score ?? scoreOf(d.rating);
		if (tmdbScore) reception.push(t('TMDB', `${(Math.round(tmdbScore * 10) / 10).toFixed(1)} / 10`));
		for (const o of outside) reception.push(t(scoreName(o.source), `${scoreText(o)}${o.max !== 100 ? ` / ${o.max}` : ''}${o.votes ? ` · ${o.votes.toLocaleString()} votes` : ''}`));
		const won = awards.filter((a) => a.won).length;
		const nominated = awards.length - won;
		if (awards.length) reception.push(t('Awards', [won ? `${won} ${won === 1 ? 'win' : 'wins'}` : '', nominated ? `${nominated} ${nominated === 1 ? 'nomination' : 'nominations'}` : ''].filter(Boolean).join(' · ')));
		if (awardBadges.length) reception.push(t('Best known for', awardBadges.join(', ')));
		const kinds: Fact[] = [];
		if (d.genres.length) kinds.push(t(d.genres.length > 1 ? 'Genres' : 'Genre', d.genres.join(', ')));
		// Themes about the story only, not the film itself (credits stingers, adaptations, sequels).
		const story = tags.filter((k) => !/stinger|^based on|sequel|remake|woman director|duringcredits|aftercredits/i.test(k));
		if (tagsAt !== 'band' && story.length) kinds.push(t('Themes', story.join(', ')));
		const groups = [
			{ title: 'Information', rows: info1 },
			{ title: 'Production', rows: made },
			{ title: 'Links', rows: links },
			{ title: 'Ratings', rows: reception },
			{ title: 'Feel', rows: kinds }
		];
		return groups.map((g) => ({ ...g, rows: g.rows.filter((f) => aboutShown(f.label, aboutPicks.list)) })).filter((g) => g.rows.length);
	});
	// Decide in 5 s: a few key facts beside the ratings; everything else goes to About at the bottom.
	const keyFacts = $derived(facts.filter((f) => f.kind === 'makers' || f.label === 'Network' || (f.label === 'Language' && info?.originalLanguage !== 'en')).slice(0, 4));
	const aboutFacts = $derived(look === 'decide' ? facts.filter((f) => !keyFacts.includes(f)) : facts);
	let aboutWidth = $state(0);
	const tagsAt = 'band' as TagsAt;
	// MUBI line: director · country · minutes.
	const mubiCaps = $derived([makers[0]?.name ?? '', countries[0] ?? '', details?.kind === 'movie' && details.runtime ? `${details.runtime}′` : lengthWord].filter(Boolean).join(' · '));
	// About band: every score this title has (first is the big one), the award count, and three fact columns.
	const allScores = $derived.by(() => {
		const out: { name: string; value: string; votes: string }[] = [];
		// Every score on one scale (out of 10, one decimal) so the rows line up and compare at a glance.
		const ten = (v: number, max: number) => ((Math.round((v / max) * 100) / 10).toFixed(1));
		for (const o of outside) out.push({ name: scoreName(o.source), value: ten(o.value, o.max), votes: o.votes ? o.votes.toLocaleString() : '' });
		const t = rich?.score ?? scoreOf(details?.rating);
		if (t) out.push({ name: 'TMDB', value: ten(t, 10), votes: '' });
		return out;
	});
	const awardWins = $derived(awards.filter((a) => a.won).length);
	const awardNames = $derived(
		[...awards.filter((a) => a.won && a.family), ...awards.filter((a) => a.won && !a.family)].slice(0, 3).map((a) => a.name.replace(/^.*?Awards? for /i, ''))
	);
	const atvCols = $derived.by(() => {
		if (!details) return [];
		const d = details;
		const i = info;
		const row = (label: string, value: string | number | undefined | null) => (value ? [{ label, value: String(value) }] : []);
		const ended = i?.status === 'Ended' || i?.status === 'Canceled';
		const eps = seasons.filter((x) => x.number > 0).reduce((n, x) => n + (x.episodes ?? 0), 0);
		const information = [
			...row(network ? 'Network' : i?.studios.length ? 'Studio' : '', network || i?.studios.slice(0, 2).join(', ')),
			...row(d.kind === 'show' ? 'Aired' : 'Released', d.kind === 'show' ? (i?.dates.firstAir ? `${i.dates.firstAir.slice(0, 4)}–${ended && i.dates.lastAir ? i.dates.lastAir.slice(0, 4) : 'now'}` : d.year) : i?.dates.theaters ? fmtDay(i.dates.theaters) : d.year),
			...row(d.kind === 'show' ? 'Seasons' : 'Run Time', d.kind === 'show' ? (regular ? `${regular} · ${eps} episodes` : '') : runtimeWords(d.runtime)),
			...row('Rated', cert),
			...row('Status', d.kind === 'show' ? statusWord(i?.status) || i?.status : ''),
			...row(countries.length > 1 ? 'Regions of Origin' : 'Region of Origin', countries.join(', '))
		].filter((r) => r.label);
		const languages = [
			...row('Original Language', lang),
			...row('Spoken Languages', (i?.spokenLanguages.length ?? 0) > 1 ? i?.spokenLanguages.join(', ') : ''),
			...row('Original Title', i?.originalTitle)
		];
		const release = [
			...row('Digital', i?.dates.digital ? fmtDay(i.dates.digital) : ''),
			...row('On Disc', i?.dates.physical ? fmtDay(i.dates.physical) : ''),
			...row('Next Episode', i?.nextEpisode ? `${fmtDay(i.nextEpisode.airDate)} · S${i.nextEpisode.season} E${i.nextEpisode.episode}` : ''),
			...row('Budget', money(i?.budget)),
			...row('Box Office', money(i?.revenue))
		];
		return [
			{ title: 'Information', rows: information },
			{ title: 'Languages', rows: languages },
			{ title: d.kind === 'show' ? 'Schedule' : 'Release & Box Office', rows: release }
		].filter((c) => c.rows.length);
	});
	// The short facts: one label, one value each.
	const parts = $derived.by((): { label: string; value: string }[] => {
		if (!details || !info) return [];
		const i = info;
		const out: { label: string; value: string }[] = [];
		if (details.kind === 'movie') {
			if (i.dates.theaters) out.push({ label: 'Released', value: fmtMonth(i.dates.theaters) });
			else if (details.year) out.push({ label: 'Released', value: String(details.year) });
			if (i.dates.digital) out.push({ label: 'Digital', value: fmtMonth(i.dates.digital) });
			if (i.budget) out.push({ label: 'Budget', value: money(i.budget) });
			if (i.revenue) out.push({ label: 'Box office', value: money(i.revenue) });
		} else {
			if (i.episodeRuntime) out.push({ label: 'Per episode', value: `${i.episodeRuntime} min` });
			const eps = seasons.filter((x) => x.number > 0).reduce((n, x) => n + (x.episodes ?? 0), 0);
			if (eps) out.push({ label: 'Episodes', value: String(eps) });
			const y1 = i.dates.firstAir?.slice(0, 4);
			const ended = i.status === 'Ended' || i.status === 'Canceled';
			if (y1) out.push({ label: 'Aired', value: ended && i.dates.lastAir ? `${y1}–${i.dates.lastAir.slice(0, 4)}` : `${y1}–now` });
			if (i.nextEpisode) out.push({ label: 'Next', value: `${fmtDay(i.nextEpisode.airDate)}` });
			else if (i.status) out.push({ label: 'Status', value: statusWord(i.status) || i.status });
		}
		if (awardBadges.length) out.push({ label: 'Awards', value: awardBadges[0] });
		if (i.originalLanguage && i.originalLanguage !== 'en') out.push({ label: 'Language', value: lang });
		if (i.originalTitle) out.push({ label: 'Original title', value: i.originalTitle });
		return out;
	});
	// The small print under the credits: each fact as "value label", rating left out (it is in the facts row).
	const fine = $derived(
		parts.map((p) =>
			['Released', 'Aired', 'Digital', 'Next'].includes(p.label) ? `${p.label} ${p.value}` : ['Status', 'Awards', 'Language', 'Original title'].includes(p.label) ? p.value : `${p.value} ${p.label.toLowerCase()}`
		)
	);
	// Themes as words for "This movie is:" (story keywords only, first four).
	const moods = $derived(tags.filter((k) => !/stinger|^based on|sequel|remake|woman director|duringcredits|aftercredits/i.test(k)).slice(0, 4));
	type Tile = { value: string; label: string };
	// Details: a few big numbers that are not already at the top, then links out.
	const tiles = $derived.by((): Tile[] => {
		if (!details || !info) return [];
		const d = details;
		const i = info;
		const out: Tile[] = [];
		const ended = i.status === 'Ended' || i.status === 'Canceled';
		if (d.kind === 'show') {
			const eps = seasons.filter((x) => x.number > 0).reduce((n, x) => n + (x.episodes ?? 0), 0);
			if (eps) out.push({ value: String(eps), label: eps === 1 ? 'episode' : 'episodes' });
			const per = i.episodeRuntime ?? 0;
			if (eps && per) out.push({ value: `~${Math.round((eps * per) / 60)} h`, label: `to watch it all, ~${per} min each` });
			// How long it ran (or has been running): 9 years on air, 2011–2019.
			const y1 = i.dates.firstAir ? Number(i.dates.firstAir.slice(0, 4)) : 0;
			const y2 = ended && i.dates.lastAir ? Number(i.dates.lastAir.slice(0, 4)) : new Date().getFullYear();
			if (y1 && y2 > y1) out.push({ value: `${y2 - y1 + 1} years`, label: ended ? `on air, ${y1}–${y2}` : `on air since ${y1}` });
			if (i.nextEpisode) out.push({ value: fmtDay(i.nextEpisode.airDate), label: `next: S${i.nextEpisode.season} E${i.nextEpisode.episode}` });
			else if (ended && i.dates.lastAir && !(y1 && y2 > y1)) out.push({ value: fmtMonth(i.dates.lastAir), label: i.status === 'Canceled' ? 'canceled' : 'last episode' });
			else if (i.status) out.push({ value: statusWord(i.status) || i.status, label: 'status' });
		} else {
			if (i.revenue) out.push({ value: money(i.revenue), label: i.budget ? `box office, ${Math.round(i.revenue / i.budget)}× its budget` : 'box office' });
			if (i.budget) out.push({ value: money(i.budget), label: 'budget' });
			if (i.dates.theaters) out.push({ value: fmtMonth(i.dates.theaters), label: 'in theaters' });
			if (i.dates.digital) out.push({ value: fmtMonth(i.dates.digital), label: 'digital release' });
			else if (i.dates.theaters && Date.parse(i.dates.theaters) > Date.now() - 120 * 864e5) out.push({ value: 'Not yet', label: 'digital release' });
		}
		return out.slice(0, 4);
	});
	// The details as one sentence (the Apple-like card).
	const summary = $derived(tiles.map((t) => `${t.value} ${t.label}`).join(' · '));
	// What a US age rating means, in a few words (the age-rating card).
	const certMeaning = (c: string) =>
		({
			G: 'For all ages',
			PG: 'Some parts may not suit young children',
			'PG-13': 'Some parts may not suit children under 13',
			R: 'Under 17 only with a parent or guardian',
			'NC-17': 'Adults only, 18 and over',
			'TV-Y': 'Made for young children',
			'TV-Y7': 'For children 7 and over',
			'TV-G': 'For all ages',
			'TV-PG': 'Parents may want to watch with children',
			'TV-14': 'May not suit children under 14',
			'TV-MA': 'For adults; may not suit anyone under 17'
		})[c] ?? '';
	// One quiet line, only when it says something: not in English, or known by another title.
	const langLine = $derived(
		[info?.originalLanguage && info.originalLanguage !== 'en' ? `In ${lang}` : '', info?.originalTitle ? `Original title: ${info.originalTitle}` : ''].filter(Boolean).join(' · ')
	);
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
	const aboutCols = $derived(Math.max(2, Math.min(aboutFacts.length, Math.floor((aboutWidth + 28) / ((aboutWidth < 600 ? 130 : 150) + 28)) || 2)));
	// Columns: as many as fit (150 px each, 130 on a phone), at least two. One item alone on the last
	// row is avoided with one column fewer; when that does not help it spans two columns (factSpan).
	let gridWidth = $state(0);
	const factFit = $derived(Math.max(2, Math.floor((gridWidth + 28) / ((gridWidth < 600 ? 130 : 150) + 28)) || 2));
	const factCols = $derived.by(() => {
		const n = facts.length;
		if (n <= factFit) return Math.max(1, n);
		for (let c = factFit; c >= Math.max(2, factFit - 1); c--) if (n % c !== 1) return c;
		return factFit;
	});
	const crumbs = $derived(details ? [{ href: to('/explore'), label: 'Explore' }, { href: to(detailPath(details.kind, details.tmdb)), label: details.year ? `${details.name} (${details.year})` : details.name }] : []);
	// The breadcrumb's parent for a page opened fresh: the title's section (Movies or TV shows).
	const sectionCrumb = $derived(details?.kind === 'show' ? { label: 'TV shows', href: to('/library?kind=show') } : { label: 'Movies', href: to('/library?kind=movie') });
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
	<article class="tp" class:old-gap={!view.wide} class:no-names={!view.names} data-testid="title-detail">
		<TitleHero
			title={d.name}
			kind={d.kind}
			tmdb={d.tmdb}
			poster={d.poster}
			backdrop={d.backdrop}
			backdrops={rich?.backdrops}
			logo={rich?.logo}
			crumbIcon={d.kind === 'show' ? 'tv' : 'compass'}
			breadcrumb={[sectionCrumb, ...crumbs.slice(1)]}
			meta={heroFacts(d)}
			heroStory={look === 'mubi' || look === 'google' ? d.overview : undefined}
			heroCaps={look === 'mubi' ? mubiCaps : undefined}
			showFacts={view.heroFacts}
			ratings={view.heroFacts}
			tagline={info?.tagline}
			certification={rich?.certification ?? certOf(d.rating)}
			score={rich?.score ?? scoreOf(d.rating)}
			status={d.neverAdd || d.ask ? heroPill(d).kind : undefined}
			statusLabel={d.neverAdd || d.ask ? heroPill(d).label : undefined}
			vtName={tmdbArtName(d.kind, d.tmdb)}
		>
			{#snippet actions()}
				{#if manage && !d.neverAdd}
					{#if d.kind === 'movie' && d.movie}
						<ActionButton icon="plus" label="Add {d.name} to the library" caption={view.newWords ? 'Add to library' : 'Add'} primary onclick={() => addMovie?.open(d.movie as LookupMovie)} />
					{:else if d.kind === 'show' && d.show}
						<ActionButton icon="plus" label="Add {d.name} to the library" caption={view.newWords ? 'Add to library' : 'Add'} primary onclick={() => addShow?.open(d.show as LookupShow)} />
					{/if}
				{:else if mayAsk && !d.neverAdd}
					{#if d.kind === 'movie'}
						<ActionButton icon="wish" label={d.ask ? `Ask for ${d.name} too` : `Ask for ${d.name}`} caption={view.newWords ? (d.ask?.mine ? 'Requested' : d.ask ? 'Request too' : 'Request') : d.ask?.mine ? 'Asked' : d.ask ? 'Ask too' : 'Ask'} primary disabled={!!d.ask?.mine} onclick={() => askMovieDialog?.open({ tmdb: d.tmdb, name: d.name, year: d.year, ask: d.ask })} />
					{:else if d.show}
						<ActionButton icon="wish" label={d.ask ? `Ask for ${d.name} too` : `Ask for ${d.name}`} caption={view.newWords ? (d.ask ? 'Request too' : 'Request') : d.ask ? 'Ask too' : 'Ask'} primary onclick={() => askShow?.open(d.show as LookupShow)} />
					{/if}
				{/if}
				{#if d.kind === 'show'}<FollowButton tmdb={d.tmdb} kind="show" />{/if}
			{/snippet}
		</TitleHero>

		{#snippet castRow()}
			{#if d.cast.length}
				<Shelf title="Cast and crew" item="112px" anchor="title-cast" whole onseeall={() => (allCast = true)}>
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
		{#snippet trailers()}
			<TitleTrailers kind={d.kind} tmdb={d.tmdb} title={d.name} videos={rich?.videos ?? []} art={[d.backdrop, ...(rich?.backdrops ?? [])]} />
		{/snippet}

		{#snippet grid(list: Fact[], cols: number)}
			<dl class="facts-grid" style="--cols: {cols}">
				{#each list as f, i (f.label)}
					<div class:span={i === list.length - 1 && list.length % cols === 1 && cols > 1}>
						<dt>{f.label}</dt>
						<dd>
							{#if f.kind === 'makers'}{@render names(makers, 3)}
							{:else if f.kind === 'cert'}<span class="fb cert">{f.value}</span>
							{:else}{f.value}{/if}
						</dd>
					</div>
				{/each}
			</dl>
		{/snippet}
		{#snippet factsLine(withGenres = true)}
			{@const parts = withGenres ? heroFacts(d) : heroFacts(d).slice(0, heroFacts(d).length - d.genres.slice(0, d.kind === 'show' ? 2 : 3).length)}
			<!-- One even row: every item, the age rating and each score apart by the same · and space. -->
			<div class="calm-meta">
				{#each parts as f, i (i)}{#if i}<span class="sep" aria-hidden="true">·</span>{/if}<span>{f}</span>{/each}
				{#if cert}<span class="sep" aria-hidden="true">·</span><span class="fb cert">{cert}</span>{/if}
				{#if !withGenres && top !== 'r5' && (topScores.length || topTmdb)}<span class="sep" aria-hidden="true">·</span><a class="top-scores" class:grey={top === 'r4'} href="#ratings" title="All ratings"><TitleScores scores={topScores} tmdb={topTmdb} variant="mono" /></a>{/if}
			</div>
		{/snippet}
		{#snippet greyCredits()}
			{#if makers.length || stars.length}
				<div class="grey-credits" data-testid="title-credits">
					{#if makers.length}<p>{madeBy} {@render names(makers, 2)}</p>{/if}
					{#if stars.length}<p>Starring {@render names(stars, 3)}</p>{/if}
				</div>
			{/if}
		{/snippet}
		{#snippet credits()}
			{#if makers.length || stars.length}
				<p class="credits" data-testid="title-credits">
					{#if makers.length}<span><span class="cl">{madeBy}</span> {@render names(makers, 2)}</span>{/if}
					{#if stars.length}<span><span class="cl">Starring</span> {@render names(stars, 3)}</span>{/if}
				</p>
			{/if}
		{/snippet}
		{#snippet tagLine()}
			{#if tagsAt === 'band' && tags.length}<ul class="tagchips quiet" data-testid="title-tags">{#each tags.slice(0, 5) as t (t)}<li>{t}</li>{/each}</ul>{/if}
		{/snippet}
		{#if look !== 'both' && look !== 'band'}
		{#if view.band === 'netflix'}
		<!-- Mix 2 · Netflix: left the facts row, story and where to watch; right the credits and the tags. -->
		<div class="side nf" data-testid="title-side">
			<div class="d-left">{@render factsLine(false)}{@render story()}</div>
			<div class="nf-right">
				{@render greyCredits()}
				{#if d.genres.length}<p class="nf-line">Genres <span>{d.genres.slice(0, 3).join(', ')}</span></p>{/if}
				{#if network}<p class="nf-line">Network <span>{network}</span></p>{/if}
				{#if tags.length}<p class="nf-line">This {d.kind === 'show' ? 'show' : 'movie'} is <span>{tags.slice(0, 4).join(', ')}</span></p>{/if}
			</div>
		</div>
		{:else if look === 'ledger' || look === 'netflix1' || look === 'google'}
		<!-- A / D / C: one calm column. A: two scores, story, grey credits, where to watch. D: story, credits (scores are in the hero). C: credits, where (story and score are in the hero). -->
		<div class="calm one" data-testid="title-calm">
			{#if !view.heroFacts}{@render factsLine(false)}{/if}
			{#if look !== 'google'}{@render story()}{/if}
			{@render greyCredits()}
			{@render tagLine()}
			
		</div>
		{:else if look === 'mubi'}
		<!-- B: everything is in the hero; only where to watch under it. -->
		{:else if look === 'decide'}
		<!-- V4 · Split: the story and where to watch beside the ratings and a few key facts. -->
		<div class="decide" data-testid="title-decide">
			<div class="d-left">{@render story()}{@render tagLine()}</div>
			{#if hasScores || keyFacts.length}
				<div class="d-right">
					{#if hasScores}<TitleScores scores={picked.scores} tmdb={picked.tmdb} variant="tiles" />{/if}
					{#if keyFacts.length}{@render grid(keyFacts, 2)}{/if}
				</div>
			{/if}
		</div>
		{:else if look === 'herostory'}
		<!-- Story in the hero: under it only where to watch. -->
		{:else if look === 'side'}
		<!-- Netflix-like: the story and scores left, short credits right. -->
		<div class="side" data-testid="title-side">
			<div class="d-left">{#if !view.heroFacts}{@render factsLine(false)}{/if}{@render story()}{@render tagLine()}</div>
			<dl class="side-credits">
				{#if makers.length}<div><dt>{madeBy}</dt><dd>{@render names(makers, 2)}</dd></div>{/if}
				{#if stars.length}<div><dt>Starring</dt><dd>{@render names(stars, 3)}</dd></div>{/if}
				{#if d.genres.length}<div><dt>Genres</dt><dd>{d.genres.slice(0, 3).join(', ')}</dd></div>{/if}
				{#if network}<div><dt>Network</dt><dd>{network}</dd></div>
				{:else if info?.studios.length}<div><dt>Studio</dt><dd>{info.studios[0]}</dd></div>{/if}
			</dl>
		</div>
		{:else}
		<!-- One calm column under the hero: Quiet, Story + credits, Scores in hero, Story first. -->
		<div class="calm" data-testid="title-calm">
			{#if look === 'story'}{@render factsLine()}{/if}
			{@render story()}
			{@render tagLine()}
			{#if look === 'credits' || look === 'scores'}{@render credits()}{/if}
			{#if look !== 'scores' && hasScores}<TitleScores scores={picked.scores} tmdb={picked.tmdb} variant={look === 'story' ? 'tiles' : 'chips'} />{/if}
			
		</div>
		{/if}
		{@render trailers()}
		{#if d.kind === 'show'}<SeasonFolds tmdb={d.tmdb} {seasons} fallback={d.backdrop} />{/if}
		{@render castRow()}
		<TitleRows kind={d.kind} tmdb={d.tmdb} bind:info band={false} />
		{#if d}
			<!-- Apple TV-like About band: a lighter strip to the page edges; a row of cards (the title, every score,
			     awards), then three columns of plain label/value facts, then links out. -->
			<section class="atv" data-testid="title-ledger">
				<h2 class="atv-h">About</h2>
				<div class="atv-cards" class:two={cardLook === 'two'} class:solo={cardLook === 'two' && !awards.length}>
					{#if cardLook === 'three'}
					<article class="atv-card">
						<h3>{d.name}</h3>
						<p class="atv-cap">{d.genres.slice(0, 2).join(', ') || (d.kind === 'show' ? 'TV show' : 'Movie')}</p>
						{#if d.overview}<p class="atv-text clamp">{d.overview}</p>{/if}
					</article>
					{/if}
					{#if allScores.length}
						<!-- Every score as one row: colored mark and name left, the score out of 10 right. -->
						<article class="atv-card">
							<h3 id="ratings">Ratings</h3>
							<p class="atv-cap">Out of 10</p>
							<div class="atv-scores" ><TitleScores scores={outside} tmdb={rich?.score ?? scoreOf(d.rating)} variant="list" outOf10 showMissing={view.missing} /></div>
						</article>
					{/if}
					{#if awards.length}
						<article class="atv-card">
							<h3>{awardBadges[0] ?? `${awardWins} wins`}</h3>
							<p class="atv-cap">Awards</p>
							{#if cardLook === 'two'}
								<!-- The awards it won, by name (major ones first), then the count of the rest. -->
								<ul class="atv-awards">
									{#each awardNames as a (a)}<li>{a}</li>{/each}
								</ul>
								<p class="atv-more">{awardWins} {awardWins === 1 ? 'win' : 'wins'} · {awards.length - awardWins} {awards.length - awardWins === 1 ? 'nomination' : 'nominations'}</p>
							{:else}
								<dl class="atv-rows">
									<div><dt>Wins</dt><dd>{awardWins}</dd></div>
									<div><dt>Nominations</dt><dd>{awards.length - awardWins}</dd></div>
								</dl>
							{/if}
						</article>
					{/if}
				</div>
				<h3 class="atv-h3">Information</h3>
				<dl class="atv-grid">{#each atvCols.flatMap((c) => c.rows) as r (r.label)}<div><dt>{r.label}</dt><dd>{r.value}</dd></div>{/each}</dl>
				<p class="atv-links">{#each outLinks as l, i (l.name)}{#if i}<span class="sep" aria-hidden="true">·</span>{/if}<a href={l.href} target="_blank" rel="noopener noreferrer">{l.name} ↗</a>{/each}</p>
			</section>
		{/if}
		{#snippet ratingCard()}
			{#if cert}<div class="ac rating-card"><span class="fb cert big">{cert}</span><span class="cap">Rated</span></div>{/if}
		{/snippet}
		{#snippet linksLine()}
			<p class="card-links">{#each outLinks as l, i (l.name)}{#if i}<span class="sep" aria-hidden="true">·</span>{/if}<a href={l.href} target="_blank" rel="noopener noreferrer">{l.name} ↗</a>{/each}</p>
		{/snippet}
		{:else}
		<!-- The story; the ratings on their own row, then every detail in one even grid; where to watch; tags; then the trailers. -->
		<div class="strip">
			{@render story()}
			{#if hasScores || facts.length}
				<div class="infoblock" data-testid="title-info">
					{#if hasScores}<div class="scores-row"><TitleScores scores={picked.scores} tmdb={picked.tmdb} variant="tiles" /></div>{/if}
					{#if facts.length}
						<dl class="facts-grid" style="--cols: {factCols}" bind:clientWidth={gridWidth} data-testid="title-facts">
							{#each facts as f, i (f.label)}
								<div class:span={i === facts.length - 1 && facts.length % factCols === 1 && factCols > 1}>
									<dt>{f.label}</dt>
									<dd>
										{#if f.kind === 'makers'}{@render names(makers, 3)}
										{:else if f.kind === 'cert'}<span class="fb cert">{f.value}</span>
										{:else if f.kind === 'awards'}<span class="awards" title={awardsTip(awards)}>{#each awardBadges as b (b)}<span class="fb award"><svg width="11" height="11" viewBox="0 0 12 12" aria-hidden="true"><path d="M3 1h6v2.4a3 3 0 0 1-6 0zM3 2H1.2v.8A2 2 0 0 0 3 4.8M9 2h1.8v.8A2 2 0 0 1 9 4.8M5.3 6.3h1.4v2.2H5.3zM3.6 8.5h4.8V11H3.6z" fill="currentColor" /></svg>{b}</span>{/each}</span>
										{:else}{f.value}{/if}
									</dd>
								</div>
							{/each}
						</dl>
					{/if}
				</div>
			{/if}
			
			{#if tags.length}
				<div class="where-row" data-testid="title-tags"><span class="lbl">Tags</span><ul class="tagchips">{#each tags as t (t)}<li>{t}</li>{/each}</ul></div>
			{/if}
		</div>
		{@render trailers()}
		{#if d.kind === 'show'}<SeasonFolds tmdb={d.tmdb} {seasons} fallback={d.backdrop} />{/if}
		{@render castRow()}
		<TitleRows kind={d.kind} tmdb={d.tmdb} bind:info band={false} />
		{/if}


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
	/* Linked names read as text until hovered. */
	.lbl { color: var(--text-muted); }
	.lead { max-width: 72ch; margin: 0; font: 400 16px/1.55 var(--font-ui); color: var(--text); }
	.svc { display: flex; flex-wrap: wrap; gap: 8px; margin: 0; padding: 0; list-style: none; }
	.svc li { display: inline-flex; align-items: center; gap: 8px; min-height: 36px; padding: 4px 12px 4px 4px; border-radius: 999px; background: var(--surface-2); font: 500 14px/1 var(--font-ui); }
	.svc img { width: 28px; height: 28px; border-radius: 7px; object-fit: cover; }
	/* The ratings on their own row, then the details in one even grid: columns line up at every width. */
	.infoblock { display: flex; flex-direction: column; border-block: 1px solid var(--line); }
	.scores-row { padding: 16px 0; }
	.scores-row + .facts-grid { border-top: 1px solid var(--line); }
	.decide { display: grid; grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr); gap: 20px 48px; align-items: start; padding-bottom: 20px; border-bottom: 1px solid var(--line); }
	.d-left, .d-right { display: flex; flex-direction: column; gap: 18px; min-width: 0; }
	.d-right .facts-grid { padding: 14px 0 0; border-top: 1px solid var(--line); }
	.calm { display: flex; flex-direction: column; gap: 16px; max-width: 760px; padding-bottom: 8px; }
	.top-scores { display: contents; color: inherit; text-decoration: none; }
	.top-scores.grey :global(.mark) { filter: grayscale(1); opacity: .75; }
	#ratings { scroll-margin-top: 90px; }
	.tp.old-gap { --tp-gap: clamp(24px, 3.4vw, 38px); }
	.tp.no-names :global(.mono .n) { display: none; }
	.calm-meta .sep { margin: 0 8px; color: var(--text-faint); }
	.credits { display: flex; flex-wrap: wrap; gap: 4px 20px; margin: -4px 0 0; color: var(--text); font: 500 14px/1.45 var(--font-ui); }
	.credits .cl, .side-credits dt { color: var(--text-muted); font-weight: 500; }
	.credits a, .side-credits a { color: inherit; }
	.side { display: grid; grid-template-columns: minmax(0, 1.6fr) minmax(0, 1fr); gap: 20px 48px; align-items: start; padding-bottom: 8px; }
	.side-credits { display: grid; gap: 10px; margin: 0; font: 500 14px/1.45 var(--font-ui); }
	.side-credits div { display: flex; flex-wrap: wrap; gap: 0 6px; }
	.side-credits dd { margin: 0; color: var(--text); }
	@media (max-width: 899px) { .side { grid-template-columns: minmax(0, 1fr); } }
	.calm-meta { margin: 0; display: flex; flex-wrap: wrap; align-items: center; gap: 6px 0; color: var(--text-muted); font: 500 15.5px/1.4 var(--font-ui); }
	.calm .tagchips.quiet, .d-left .tagchips.quiet { margin-top: -6px; }
	.fb.cert.big { height: 30px; padding: 0 10px; font-size: 16px; border-radius: 7px; }
	.calm.one { max-width: 820px; gap: 14px; }
	.nf-right { display: grid; gap: 8px; align-content: start; }
	.nf-line { margin: 0; color: var(--text-muted); font: 400 14px/1.5 var(--font-ui); }
	.nf-line span { color: var(--text); }
	.calm.one .lead { font-size: 17px; line-height: 1.5; }
	.grey-credits { display: grid; gap: 2px; color: var(--text-muted); font: 400 14px/1.5 var(--font-ui); }
	.grey-credits p { margin: 0; }
	.grey-credits a { color: var(--text); text-decoration: none; }
	/* About band, Apple TV-like: lighter strip edge to edge, cards, then label/value columns. */
	/* About band, measured on tv.apple.com (1440-1650 px desktop): band one step lighter than the page (#2b2b2b on #1f1f1f),
	   cards white 10% with 12 px corners and 14/16/16 padding, 20 px gaps; 17 px bold titles, 11 px caps, 15/20 text, 12 px facts. */
	.atv { position: relative; margin-top: 32px; padding: 32px 0 40px; background: var(--atv-band); box-shadow: 0 0 0 100vmax var(--atv-band); clip-path: inset(0 -100vmax -100vmax -100vmax); --atv-band: color-mix(in srgb, var(--text) 7%, var(--bg)); --atv-card: rgb(255 255 255 / 10%); }
	:global([data-theme='light']) .atv { --atv-band: color-mix(in srgb, var(--text) 5%, var(--bg)); --atv-card: var(--bg); }
	.atv-scores :global(.list li) { padding: 0; border-top: 0; font-size: 15px; line-height: 20px; }
	.atv-scores :global(.list .n) { color: var(--text-muted); font: 400 15px/20px var(--font-ui); }
	.atv-scores :global(.list .v) { color: var(--text-muted); font: 400 15px/20px var(--font-ui); font-variant-numeric: tabular-nums; }
	.atv-h { margin: 0 0 13px; color: var(--text); font: 700 17px/21px var(--font-ui); }
	.atv-cards { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 20px; }
	.atv-card { min-width: 0; padding: 14px 16px 16px; border-radius: 12px; background: var(--atv-card); }
	/* Cards keep a third of the width each, however many there are (never one stretched card). */
	.atv-awards { display: grid; gap: 0; margin: 0; padding: 0; list-style: none; color: var(--text-muted); font: 400 15px/20px var(--font-ui); }
	.atv-more { margin: 6px 0 0; color: var(--text-muted); font: 400 13px/16px var(--font-ui); }
	.atv-card h3 { margin: 0; padding-bottom: 5px; color: var(--text); font: 700 17px/21px var(--font-ui); }
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
	.atv-links a:hover { color: var(--text); }
	@media (max-width: 899px) { .atv-cards, .atv-grid { grid-template-columns: minmax(0, 1fr); } }
	.card-links { display: flex; flex-wrap: wrap; align-items: center; margin: 14px 0 0; color: var(--text-muted); font: 500 14px/1.4 var(--font-ui); }
	.card-links a { color: var(--text); text-decoration: none; }
	.card-links a:hover { text-decoration: underline; }
	.card-links .sep { margin: 0 8px; color: var(--text-faint); }
	.card-links a { color: var(--text); text-decoration: none; }
	.card-links a:hover { text-decoration: underline; }
	.tagchips.quiet li { background: none; border: 1px solid var(--line); color: var(--text-muted); font-size: 12px; }
	@media (max-width: 899px) { .decide { grid-template-columns: minmax(0, 1fr); } }
	.facts-grid { display: grid; grid-template-columns: repeat(var(--cols, 4), minmax(0, 1fr)); gap: 16px 28px; margin: 0; padding: 16px 0; }
	.facts-grid > div { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
	.facts-grid > div.span { grid-column: span 2; }
	.facts-grid dt { color: var(--text-muted); font: 600 11.5px/1.2 var(--font-ui); letter-spacing: .06em; text-transform: uppercase; }
	.facts-grid dd { margin: 0; color: var(--text); font: 500 15px/1.35 var(--font-ui); overflow-wrap: anywhere; }
	.tp :global(.facts-grid a) { color: inherit; text-decoration: none; }
	.tp :global(.facts-grid a:hover) { text-decoration: underline; text-underline-offset: 3px; }
	.strip { display: flex; flex-direction: column; gap: 20px; }
	.fb { display: inline-flex; align-items: center; gap: 4px; height: 22px; padding: 0 7px; border-radius: 5px; border: 1.5px solid currentColor; font: 700 11.5px/1 var(--font-ui); letter-spacing: .03em; white-space: nowrap; }
	.awards { display: flex; flex-wrap: wrap; gap: 6px; }
	.fb.award { border-color: rgb(196 150 10 / 70%); color: #a77c00; }
	:global([data-theme='dark']) .fb.award { color: #f5c518; border-color: rgb(245 197 24 / 70%); }
	.tagchips { display: flex; flex-wrap: wrap; gap: 6px; margin: 0; padding: 0; list-style: none; }
	.tagchips li { display: inline-flex; align-items: center; min-height: 30px; padding: 0 12px; border: 1px solid var(--line); border-radius: 999px; color: var(--text); font: 500 13px/1 var(--font-ui); }
	.where-row { display: flex; flex-wrap: wrap; align-items: center; gap: 10px 14px; font: 500 14px/1 var(--font-ui); }
</style>
