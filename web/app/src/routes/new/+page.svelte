<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { to } from '#lib/paths.ts';
	import { detailPath, loadDetails, remember } from '#lib/title-details.ts';
	import { api, message } from '#lib/api.ts';
	import { session } from '#lib/session.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { refreshInbox } from '#lib/wishlist.svelte.ts';
	import { cardAction, cardTmdb, discoverCard, liveCard, matchOwned, ownedIndex, ownedMark, ownedTitle, serverRow, serviceRegion, type ExploreCard, type ExploreRow, type WatchService } from '#lib/explore.ts';
	import { cardServices, hasServiceChips, isRanked, newPath, newServicePath, parseNewType, pickService, type NewAnswer, type NewServiceAnswer, type NewType, type RailDay, type RailItem } from '#lib/new-popular.ts';
	import { contextLine } from '#lib/grow-lines.ts';
	import { defaultRegion } from '#lib/region.ts';
	import type { Ask, AskState, GateDefaults, LibraryFolder, LookupShow, PickRulesView, ShowSettings, TitleView } from '#lib/types.ts';
	import { seasonWords } from '#lib/shows.ts';
	import { audienceOf } from '#lib/title-status.ts';
	import { savedDefaults } from '#lib/title-defaults.svelte.ts';
	import AddMovieDialog from '#lib/AddMovieDialog.svelte';
	import AddShowDialog from '#lib/AddShowDialog.svelte';
	import AskShowDialog from '#lib/AskShowDialog.svelte';
	import AskMovieDialog from '#lib/AskMovieDialog.svelte';
	import MetadataNotice from '#lib/MetadataNotice.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import NewRail from '#lib/ui/NewRail.svelte';
	import MusicNewTab from '#lib/MusicNewTab.svelte';
	import ExploreHero from '#lib/ui/ExploreHero.svelte';
	import { whyShown, type Why } from '#lib/why-shown.ts';
	import PosterCard from '#lib/ui/PosterCard.svelte';
	import WideCard from '#lib/ui/WideCard.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import { Silhouette, SkHero, SkRail, SkWide } from '#lib/ui/sk/index.ts';
	import { buildingStatus } from '#lib/silhouette.ts';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import type { IconName } from '#lib/Icon.svelte';

	type Row = ExploreRow & { wide: boolean; type: string };

	const manage = $derived(session.can('manage_library'));
	const mayAsk = $derived(session.can('ask'));
	const who = $derived(audienceOf(session.can));

	let type = $state<NewType>(parseNewType(page.url.searchParams.get('type') ?? page.url.searchParams.get('kind')));
	const typeOptions: { value: NewType; label: string; icon: IconName }[] = [
		{ value: 'all', label: 'All', icon: 'grid' },
		{ value: 'shows', label: 'TV shows', icon: 'tv' },
		{ value: 'movies', label: 'Movies', icon: 'film' },
		{ value: 'music', label: 'Music', icon: 'music' }
	];

	let rows = $state<Row[]>([]);
	let rail = $state<RailDay[]>([]);
	let hero = $state<ExploreCard[]>([]);
	let heroAt = $state(0);
	let loaded = $state(false);
	let failed = $state(false);
	let more = $state(false);
	let next = 0;
	let busy = false;
	let ask = 0;
	let poll: ReturnType<typeof setTimeout> | undefined;
	/** The try number while the server says the lists are still building (0 = not building). */
	let building = $state(0);
	const gone = new AbortController();

	// The pills float once their spot scrolls under the top bar (as on Explore).
	let typesMark = $state<HTMLElement>();
	let typesStuck = $state(false);
	$effect(() => {
		if (!typesMark) return;
		const top = parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--topbar')) || 56;
		const io = new IntersectionObserver(([e]) => (typesStuck = !e.isIntersecting), { rootMargin: `-${top}px 0px 0px 0px` });
		io.observe(typesMark);
		return () => io.disconnect();
	});

	function setType(t: NewType) {
		type = t;
		svcLists = new Map();
		svcGen++;
		svcPicks = new Map();
		const url = new URL((page.shallow?.url ?? page.url).href);
		if (t === 'all') url.searchParams.delete('type');
		else url.searchParams.set('type', t);
		url.searchParams.delete('kind');
		void goto(url, { replace: true, shallow: true, reset: false });
		if (t !== 'music') void loadFirst();
	}

	/** loadFirst reads page 0; while lists are still building it asks again every 3 s (at most a minute). */
	async function loadFirst(tries = 0) {
		const n = ++ask;
		clearTimeout(poll);
		try {
			const a = await api.get<NewAnswer>(newPath(type, 0), gone.signal);
			if (n !== ask) return;
			rows = a.rows.map(serverRow);
			rail = a.rail ?? [];
			hero = (a.hero ?? []).map((d) => ({ ...discoverCard(d, 'np_hero'), wide: d.backdrop ?? null }));
			heroAt = nextHero(hero.length);
			next = 1;
			more = a.more;
			failed = false;
			building = a.building && tries < 20 ? tries + 1 : 0;
			if (building) poll = setTimeout(() => void loadFirst(tries + 1), 3000);
		} catch {
			if (n === ask) failed = true;
		} finally {
			if (n === ask) loaded = true;
		}
	}

	async function loadMore() {
		if (busy || !more) return;
		busy = true;
		const n = ask;
		try {
			const a = await api.get<NewAnswer>(newPath(type, next), gone.signal);
			if (n !== ask) return;
			const have = new Set(rows.map((r) => r.id));
			rows = [...rows, ...a.rows.map(serverRow).filter((r) => !have.has(r.id))];
			next++;
			more = a.more;
		} catch {
			more = false;
		} finally {
			busy = false;
		}
		requestAnimationFrame(() => {
			if (more && sentinel && sentinel.getBoundingClientRect().top < innerHeight * 2.5) void loadMore();
		});
	}

	let sentinel = $state<HTMLElement>();
	$effect(() => {
		if (!sentinel) return;
		const io = new IntersectionObserver(([e]) => e.isIntersecting && void loadMore(), { rootMargin: '150% 0px' });
		io.observe(sentinel);
		return () => io.disconnect();
	});

	// Service chips on the services and hot and new rows: one service at a time, or all (per row).
	let svcPicks = $state(new Map<string, number | null>());
	const svcPick = (row: string) => svcPicks.get(row) ?? null;
	// Each chip loads its service's own list (row type:service -> cards; null while loading).
	let svcLists = $state(new Map<string, ExploreCard[] | null>());
	let svcGen = 0;
	function setPick(row: Row, s: number | null) {
		svcPicks = new Map([...svcPicks, [row.id, s]]);
		if (s === null) return;
		const k = `${row.type}:${s}`;
		if (svcLists.has(k)) return;
		svcLists = new Map([...svcLists, [k, null]]);
		const n = svcGen;
		api.get<NewServiceAnswer>(newServicePath(type, s, row.type), gone.signal)
			.then((a) => {
				if (n === svcGen) svcLists = new Map([...svcLists, [k, serverRow(a.row).cards]]);
			})
			.catch(() => {
				if (n !== svcGen) return;
				// Fall back to the shared row's cards for that service.
				svcLists = new Map([...svcLists, [k, pickService(row.cards, s)]]);
			});
	}
	const ghosts = (n: number): ExploreCard[] => Array.from({ length: n }, (_, i) => ({ key: `ghost-${i}`, kind: 'movie', name: '', badges: [] }) as ExploreCard);
	const region = serviceRegion(defaultRegion());
	let services = $state<WatchService[]>([]);
	const logo = (id: number | undefined) => services.find((s) => s.id === id)?.logo;

	let titles = $state<TitleView[]>([]);
	const owned = $derived(ownedIndex(titles));
	let askedHere = $state(new Map<string, { ask?: AskState; titleId?: number }>());
	let asking = $state('');
	function live(c: ExploreCard): ExploreCard {
		return matchOwned(liveCard(c, askedHere.get(c.key)), owned);
	}
	async function loadTitles() {
		try {
			titles = (await api.get<{ titles: TitleView[] }>('/titles')).titles ?? [];
		} catch {
			/* owned marks wait for the Library */
		}
	}

	function cardHref(c: ExploreCard): string | undefined {
		if (c.titleId !== undefined) return to(`/library/${c.titleId}`);
		const tmdb = c.ref?.startsWith('tmdb:') ? c.ref.slice(5) : '';
		if (tmdb && (c.kind === 'movie' || c.kind === 'show')) {
			remember(c.kind, tmdb, { name: c.name, year: c.year, poster: c.image });
			return to(detailPath(c.kind, tmdb));
		}
		return undefined;
	}
	function searchHref(c: ExploreCard): string {
		return to(`/search?q=${encodeURIComponent(c.name)}${c.kind === 'show' ? '&kind=show' : ''}`);
	}

	let folders = $state<LibraryFolder[]>([]);
	let rules = $state<PickRulesView[]>([]);
	let gateDefault = $state('released');
	let showDefaults = $state<ShowSettings | null>(null);
	let addMovie: ReturnType<typeof AddMovieDialog> | undefined = $state();
	let addShow: ReturnType<typeof AddShowDialog> | undefined = $state();
	let askShow: ReturnType<typeof AskShowDialog> | undefined = $state();
	let askMovie: ReturnType<typeof AskMovieDialog> | undefined = $state();
	let current: ExploreCard | null = null;

	async function quick(c: ExploreCard, act: 'add' | 'ask') {
		const tmdb = cardTmdb(c);
		if (!tmdb || asking) return;
		current = c;
		if (act === 'ask' && c.kind === 'movie') return askMovie?.open({ tmdb, name: c.name, year: c.year, ask: c.ask });
		asking = c.key;
		try {
			const d = await loadDetails(api.get, c.kind as 'movie' | 'show', tmdb);
			if (!d) return void toasts.push(`Could not find ${c.name}. Try Search.`, 'fail');
			if (d.titleId) return void toasts.push(`${c.name} is in your Library already.`, 'ok');
			if (act === 'add' && d.movie) addMovie?.open(d.movie);
			else if (act === 'add' && d.show) addShow?.open(d.show);
			else if (d.show) askShow?.open(d.show);
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			asking = '';
		}
	}
	function askedToast(r: { ask: Ask; joined: boolean }, what: string) {
		if (current) askedHere = new Map([...askedHere, [current.key, { ask: r.ask.state }]]);
		if (r.ask.state === 'approved') toasts.push(`${what} is approved; HolyArrs is looking for it.`, 'ok');
		else if (r.joined) toasts.push(`Someone asked for ${what} already; you are on the ask too.`, 'ok');
		else toasts.push(`Asked for ${what}. You will hear when it is approved.`, 'ok');
		void refreshInbox();
	}
	function refused(body: Record<string, unknown>) {
		if (current && typeof body.title_id === 'number') askedHere = new Map([...askedHere, [current.key, { titleId: body.title_id }]]);
	}
	function added(t: TitleView) {
		toasts.push(`${t.name} is in your Library.`, 'ok');
		void loadTitles();
	}

	$effect(() => {
		if (!manage) return;
		api.get<{ library_folders: LibraryFolder[] }>('/library-folders').then((r) => (folders = r.library_folders)).catch(() => {});
		api.get<{ pick_rules: PickRulesView[] }>('/pick-rules').then((r) => (rules = r.pick_rules)).catch(() => {});
		api.get<GateDefaults>('/settings/release-gate').then((r) => (gateDefault = r.release_gate)).catch(() => {});
		api.get<ShowSettings>('/settings/shows').then((r) => (showDefaults = r)).catch(() => {});
		savedDefaults.forget();
		void savedDefaults.load();
	});

	onMount(() => {
		void loadTitles();
		if (type !== 'music') void loadFirst();
		api.get<{ services: WatchService[] }>(`/watch-services?region=${region}`, gone.signal).then((r) => (services = r.services ?? [])).catch(() => {});
		return () => {
			clearTimeout(poll);
			gone.abort();
		};
	});

	const wideKind = (k: string): boolean => k === 'movie' || k === 'show';
	function shown(row: Row): ExploreCard[] {
		if (!hasServiceChips(row.type)) return row.cards;
		const s = svcPick(row.id);
		if (s === null) return isRanked(row.type) ? row.cards.slice(0, 10) : row.cards;
		const got = svcLists.get(`${row.type}:${s}`);
		if (!got) return ghosts(isRanked(row.type) ? 6 : 8);
		return isRanked(row.type) ? got.slice(0, 10) : got;
	}
	/** railCard is a rail title as a row card (the same card and hover preview as every row). */
	const railCard = (it: RailItem): ExploreCard => ({ ...discoverCard(it, 'np_rail'), wide: it.backdrop ?? null });

	/** nextHero rotates the hero through its titles, one step a visit. */
	function nextHero(n: number): number {
		if (n < 2) return 0;
		try {
			const at = (Number(sessionStorage.getItem('np-hero-at') ?? '-1') + 1) % n;
			sessionStorage.setItem('np-hero-at', String(at));
			return at;
		} catch {
			return 0;
		}
	}
	/** heroWhy: New season, the title's place on its service's chart, else New this week. */
	function heroWhy(h: ExploreCard): Why[] {
		const out = whyShown({ kind: h.kind, tmdb: cardTmdb(h) ?? '', fresh: h.fresh });
		const rank = h.streaming?.rank;
		if (rank && rank <= 10 && h.streaming && !out.some((w) => w.kind === 'top10')) out.push({ kind: 'top10', text: `#${rank} on ${h.streaming.name}` });
		if (!out.length && h.fresh && h.fresh.kind !== 'coming') out.unshift({ kind: 'new_season', text: 'New this week' });
		return out.slice(0, 2);
	}
</script>

<svelte:head><title>New & Popular · HolyArrs</title></svelte:head>

{#snippet wideCard(raw: ExploreCard, rowType: string, rowTitle: string)}
	{@const c = live(raw)}
	{@const act = cardAction(c, { manage, ask: mayAsk })}
	{@const mark = ownedMark(c, ownedTitle(c, owned), who)}
	{@const preview = cardTmdb(c) && (c.kind === 'movie' || c.kind === 'show') ? { kind: c.kind, tmdb: cardTmdb(c), year: c.year, titleId: c.titleId } : undefined}
	<div class="logo-wrap">
		<WideCard cover title={c.name} href={cardHref(c)} image={c.wide ?? c.image} art={c.art} kind={c.kind} subtitle={c.subtitle} badges={c.badges} badgesAt="bottom" vtName={c.vtName} fresh={c.fresh} owned={mark} typed row={rowType} {preview}
			growService={c.streaming ? { name: c.streaming.name, logo: logo(c.streaming.service) } : null}
			growContext={contextLine({ state: mark?.state, label: mark?.label, mine: c.mine, fresh: c.fresh }, { rowType, rowTitle, fresh: c.fresh, streaming: c.streaming })}
			followTmdb={!c.titleId && (c.kind === 'show' || c.fresh?.kind === 'coming' || (!!c.year && c.year > new Date().getFullYear()))}>
			{#snippet growAction()}{@render hoverAction(c, act, mark)}{/snippet}
		</WideCard>
		{#if (rowType === 'np_services' || rowType === 'np_rail') && c.streaming && logo(c.streaming.service)}
			<img class="svc-logo" src={logo(c.streaming.service)} alt="On {c.streaming.name}" title={c.streaming.name} loading="lazy" />
		{/if}
	</div>
{/snippet}

{#snippet hoverAction(c: ExploreCard, act: 'add' | 'ask' | null, mark: ReturnType<typeof ownedMark>)}
	{#if act && cardTmdb(c)}
		<IconButton icon="plus" label={act === 'add' ? `Add ${c.name}` : `Ask for ${c.name}`} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={asking === c.key || undefined} onclick={() => void quick(c, act)} />
	{:else if mark?.state === 'available' || mark?.state === 'partial' || mark?.state === 'downloading'}
		<IconButton icon="check" label="In Library" variant="glass" size="lg" href={cardHref(c) ?? searchHref(c)} />
	{:else if mark?.state === 'requested'}
		<IconButton icon="check" label="Asked" variant="glass" size="lg" href={cardHref(c) ?? searchHref(c)} />
	{:else}
		<IconButton icon="play" label="Open {c.name}" variant="glass" size="lg" href={cardHref(c) ?? searchHref(c)} />
	{/if}
{/snippet}

<div class="np">
	<h1 class="sr-only">New & Popular</h1>
	<div class="types-mark" bind:this={typesMark} aria-hidden="true"></div>
	<div class="types" class:stuck={typesStuck} data-pin-pills>
		<SegmentedControl options={typeOptions} value={type} label="Show" testid="new-type" announce={(l) => `Showing ${l}`} onchange={setType} />
	</div>
	<MetadataNotice />

	{#if type === 'music'}
		<MusicNewTab {manage} {mayAsk} {folders} {rules} />
	{:else if failed && rows.length === 0}
		<LoadError hasData={false} message="Could not load New & Popular." onretry={() => void loadFirst()} busy={false} />
	{:else if !loaded || (building && rows.length === 0)}
		<!-- New & Popular-shaped placeholder: hero, the This week rail, then wide rows. "Building" answers show the try count. -->
		<Silhouette loading={true} label="Loading New & Popular" status={building ? buildingStatus(building, 'New & Popular') : 'Loading New & Popular…'} onretry={() => void loadFirst()}>
			{#snippet skeleton()}
				<div class="np-sk" data-testid="new-skeleton">
					<SkHero />
					<div class="rail-sk">
						<span class="sk sk-line" style="width: 96px; margin: 0 20px 10px"></span>
						<div class="rail-track">
							<span class="when"><span class="sk" style="height: 28px; width: 56px"></span><span class="sk sk-line" style="width: 48px; height: 10px"></span></span>
							{#each Array.from({ length: 6 }, (_, i) => i) as i (i)}<span class="rail-t"><SkWide /><span class="sk sk-line" style="width: 70%; height: 10px"></span></span>{/each}
						</div>
					</div>
					<SkRail kind="wide" item="var(--title-item)" count={6} titleWidth="200px" />
					<SkRail kind="wide" item="var(--title-item)" count={6} titleWidth="160px" />
				</div>
			{/snippet}
		</Silhouette>
	{:else if rows.length === 0}
		<EmptyState icon="compass" title="New & Popular is getting ready" text="Rows fill in within a few minutes." action={{ label: 'Explore', href: to('/explore'), icon: 'compass' }} />
	{:else}
		{#if hero.length}
			{@const h = live(hero[heroAt % hero.length])}
			<ExploreHero kind={h.kind === 'show' ? 'show' : 'movie'} tmdb={cardTmdb(h)} title={h.name} href={cardHref(h)} backdrop={h.wide ?? h.image} vtName={h.vtName} subtitle={h.year ? `${h.year} · ${h.kind === 'show' ? 'Show' : 'Movie'}` : h.subtitle} settled={loaded}>
				{#snippet action()}{@render hoverAction(h, cardAction(h, { manage, ask: mayAsk }), null)}{/snippet}
				{#snippet badge()}
					{@const why = heroWhy(h)}
					{#if why.length}<ul class="why" aria-label="Why it is featured">{#each why as w (w.text)}<li class="why-{w.kind}">{w.text}</li>{/each}</ul>{/if}
				{/snippet}
			</ExploreHero>
		{/if}
		{#if rail.length > 0}
			<NewRail days={rail}>
				{#snippet card(it: RailItem)}{@render wideCard(railCard(it), 'np_rail', 'This week')}{/snippet}
			</NewRail>
		{/if}
		{#each rows as row (row.id)}
			{@const cards = shown(row)}
			{@const wide = !isRanked(row.type) && row.cards.some((c) => wideKind(c.kind))}
			{@const chips = hasServiceChips(row.type) ? cardServices(row.cards) : []}
			<div class="row-wrap" class:wide-row={wide} data-row={row.type} data-row-id={row.id}>
				{#snippet tools()}
					<div class="chips" role="group" aria-label="Services">
						<Chip label="All" selected={svcPick(row.id) === null} onclick={() => setPick(row, null)} />
						{#each chips as s (s.service)}<Chip label={s.name} selected={svcPick(row.id) === s.service} onclick={() => setPick(row, s.service)} />{/each}
					</div>
				{/snippet}
				<Shelf title={row.title} subtitle={row.subtitle} tools={chips.length > 1 ? tools : undefined}
					item={isRanked(row.type) ? 'calc(var(--explore-item) / 0.68)' : wide ? 'var(--title-item)' : 'var(--explore-item)'}
					items={cards} key={(c) => c.key} placeholder={wide ? 'wide' : 'poster'} lines={cards.some((c) => c.subtitle) ? 2 : 1}>
					{#snippet card(raw: ExploreCard)}
						{#if raw.key.startsWith('ghost-')}
							<div class="ghost" class:pc-ghost={isRanked(row.type)}><Skeleton shape={wide ? 'wide' : 'poster'} /><Skeleton width="70%" /></div>
						{:else}
						{@const c = live(raw)}
						{@const act = cardAction(c, { manage, ask: mayAsk })}
						{@const mark = ownedMark(c, ownedTitle(c, owned), who)}
						{@const preview = cardTmdb(c) && (c.kind === 'movie' || c.kind === 'show') ? { kind: c.kind, tmdb: cardTmdb(c), year: c.year, titleId: c.titleId } : undefined}
						{#if isRanked(row.type)}
							{@const rank = cards.indexOf(raw) + 1}
							<div class="ranked">
								<span class="rank" class:two={rank > 9} aria-hidden="true">{rank}</span>
								<div class="pc"><PosterCard title={c.name} href={cardHref(c)} image={c.image} kind={c.kind} owned={mark} fresh={c.fresh} {preview} /></div>
								<span class="sr-only">Number {rank}</span>
							</div>
						{:else if wide}
							{@render wideCard(raw, row.type, row.title)}
						{:else}
							<PosterCard title={c.name} href={cardHref(c)} image={c.image} kind={c.kind} subtitle={c.subtitle} badges={c.badges} typed row={row.type} owned={mark} fresh={c.fresh} actionOnHover vtName={c.vtName} {preview}>
								{#snippet action()}
									{#if act && cardTmdb(c)}<IconButton icon="plus" label={act === 'add' ? `Add ${c.name}` : `Ask for ${c.name}`} variant="glass" size="lg" aria-haspopup="dialog" onclick={() => void quick(c, act)} />{/if}
								{/snippet}
							</PosterCard>
						{/if}
						{/if}
					{/snippet}
				</Shelf>
			</div>
		{/each}
		{#if more}<div class="sentinel" bind:this={sentinel} aria-hidden="true"></div>{/if}
	{/if}
</div>

{#if manage}
	<AddMovieDialog bind:this={addMovie} {folders} {rules} {gateDefault} onadded={(_m, t) => added(t)} />
	<AddShowDialog bind:this={addShow} {folders} {rules} defaults={showDefaults} onadded={(_s, t) => added(t)} />
{:else if mayAsk}
	<AskMovieDialog bind:this={askMovie} onasked={(_m, r) => askedToast(r, r.ask.name)} onrefused={(_m, body) => refused(body)} />
	<AskShowDialog bind:this={askShow} onasked={(_s: LookupShow, r) => askedToast(r, `${r.ask.name} (${seasonWords(r.ask.seasons)})`)} onrefused={(_s: LookupShow, body: Record<string, unknown>) => refused(body)} />
{/if}

<style>
	/* The same sizes and type as Explore. */
	.np { container: explore / inline-size; --title-item: max(200px, calc((100vw - 96px) / 5.16)); --explore-item: 168px; display: flex; flex-direction: column; gap: 32px; padding-bottom: 48px; }
	.types-mark { height: 0; margin-top: -32px; }
	.types { position: sticky; top: calc(var(--topbar) + 10px); z-index: 20; display: flex; justify-content: center; margin-top: -12px; pointer-events: none; }
	.types :global(.ui-seg) { pointer-events: auto; border-radius: 999px; padding: 3px; transition: transform 320ms cubic-bezier(.2,.8,.2,1), box-shadow 320ms ease, background-color 320ms ease; transform-origin: top center; }
	.types.stuck :global(.ui-seg) { transform: scale(0.9); background: color-mix(in srgb, var(--surface-2) 70%, transparent); -webkit-backdrop-filter: saturate(180%) blur(20px); backdrop-filter: saturate(180%) blur(20px); box-shadow: 0 10px 30px rgb(0 0 0 / 0.28), 0 0 0 0.5px color-mix(in srgb, var(--text) 14%, transparent); }
	.types :global(.ui-seg .thumb) { top: 3px; bottom: 3px; left: 3px; width: calc((100% - 6px) / var(--n)); border-radius: 999px; }
	.types :global(.ui-seg button) { min-height: 38px; padding: 0 16px; border-radius: 999px; font-size: 14px; }
	@media (prefers-reduced-motion: reduce) { .types :global(.ui-seg), .types :global(.ui-seg .thumb) { transition: none; } }
	@media (max-width: 767px) { .types :global(.ui-seg) { display: grid; width: 100%; } .types :global(.ui-seg button) { padding: 0 8px; } }
	.row-wrap { display: contents; }
	@media (min-width: 768px) { .row-wrap.wide-row { --shelf-gap: 5px; } }
	.chips { display: flex; gap: 8px; overflow-x: auto; scrollbar-width: none; margin-inline: calc(var(--gutter) * -1); padding: 2px var(--gutter); }
	.chips::-webkit-scrollbar { display: none; }
	.chips :global(.ui-chip) { flex: none; }
	.logo-wrap { position: relative; }
	.svc-logo { position: absolute; top: 8px; left: 8px; width: 28px; height: 28px; border-radius: 7px; object-fit: cover; box-shadow: 0 2px 8px rgb(0 0 0 / 0.35); pointer-events: none; z-index: 2; }
	.ranked { position: relative; container-type: inline-size; }
	.rank { position: absolute; left: 0; top: 0; width: 37%; height: 102cqw; display: flex; align-items: flex-end; justify-content: flex-end; font-size: 50cqw; font-weight: 800; line-height: 0.76; letter-spacing: -0.06em; color: var(--bg, #000); -webkit-text-stroke: 3px var(--text-2, #9a9a9a); paint-order: stroke; user-select: none; z-index: 0; }
	.rank.two { font-size: 36cqw; letter-spacing: -0.12em; }
	.pc { position: relative; z-index: 1; margin-left: 32%; width: 68%; min-width: 0; }
	.sentinel { height: 1px; }
	/* Ghost cards while a chip's list loads, the same size as the cards they stand for. */
	.ghost { display: flex; flex-direction: column; gap: 8px; }
	.ghost.pc-ghost { margin-left: 32%; width: 68%; }
	/* The rail's cards are as wide as a Hot and new card. */
	.np { --rail-item: calc(var(--explore-item) / 0.68); }
	/* Hero badges (as on Explore): small glass pills, the chart one in the brand red. */
	.why { display: flex; flex-wrap: wrap; gap: 6px; margin: 8px 0 0; padding: 0; list-style: none; }
	.why li { font: 600 12px/1 var(--font-ui); letter-spacing: .02em; padding: 6px 10px; border-radius: 999px; color: #fff; background: rgb(255 255 255 / 14%); border: 1px solid rgb(255 255 255 / 22%); -webkit-backdrop-filter: blur(14px); backdrop-filter: blur(14px); }
	.why li.why-top10 { background: color-mix(in srgb, var(--accent) 80%, transparent); border-color: transparent; }
	.np-sk { display: flex; flex-direction: column; gap: 32px; min-width: 0; }
	.rail-sk { border-radius: 20px; padding: 16px 0 14px; background: color-mix(in srgb, var(--surface-2) 72%, transparent); overflow: hidden; }
	.rail-track { display: flex; gap: var(--shelf-gap, 12px); padding: 4px 20px; }
	.rail-track .when { flex: none; width: 74px; display: flex; flex-direction: column; gap: 6px; }
	.rail-t { flex: none; width: var(--rail-item, 240px); display: flex; flex-direction: column; gap: 6px; }
	@media (max-width: 767px) { .np { --explore-item: 54vw; --title-item: calc((100cqi - var(--shelf-gap)) / 1.6 / .78); gap: 26px; } }
</style>
