<script lang="ts">
	import { dragScroll } from '#lib/drag-scroll.ts';
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api, message } from '#lib/api.ts';
	import { session } from '#lib/session.svelte.ts';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Shelf from '#lib/ui/Shelf.svelte';
	import PosterCard from '#lib/ui/PosterCard.svelte';
	import PersonCircle from '#lib/ui/PersonCircle.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import Icon from '#lib/Icon.svelte';
	import AskArtistDialog from '#lib/AskArtistDialog.svelte';
	import AddArtistDialog from '#lib/AddArtistDialog.svelte';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { refreshInbox } from '#lib/wishlist.svelte.ts';
	import { to } from '#lib/paths.ts';
	import { plainReason } from '#lib/plain.ts';
	import { cameFrom, fromIcon } from '#lib/came-from.svelte.ts';
	import { albumYear } from '#lib/music/music.ts';
	import { SkRail, SkTrackRow } from '#lib/ui/sk/index.ts';
	import ArtPlaceholder from '#lib/ui/ArtPlaceholder.svelte';
	import { pictureOf } from '#lib/pictures.ts';
	import { albumHref, appPicture, artistHref, coverFor, kindWord, length, owned, type AlbumCoreData, type AlbumPageData, type PageAlbum, type PageArtist, type PageCredit, type PageTrack } from '#lib/music/music-pages.ts';
	import type { Ask, LibraryFolder, LookupArtist, MusicSettings, PickRulesView } from '#lib/types.ts';

	// An album's page: the cover beside its title and actions, the tracklist
	// with what the Library has, its other editions, and where to go next.
	const mbid = $derived((page.params.mbid ?? '').toLowerCase());
	const artist = $derived((page.url.searchParams.get('artist') ?? '').toLowerCase());
	// Hero first: the core answer draws the cover, name and artist; the full
	// answer (tracks, editions, more by) and the similar artists follow.
	let core = $state<AlbumCoreData | null>(null);
	let data = $state<AlbumPageData | null>(null);
	let similar = $state<PageArtist[] | null>(null);
	let error = $state('');
	let busy = $state(false);
	let lookup = $state<LookupArtist | null>(null);
	let coverIn = $state(false);
	let folders = $state<LibraryFolder[]>([]);
	let rules = $state<PickRulesView[]>([]);
	let defaults = $state<MusicSettings | null>(null);
	let askDialog: ReturnType<typeof AskArtistDialog> | undefined = $state();
	let addDialog: ReturnType<typeof AddArtistDialog> | undefined = $state();
	let askedHere = $state(false);
	let near = $state<PageCredit[] | null>(null);
	let edition = $state('');
	let asking = $state(false);

	const manage = $derived(session.can('manage_library'));
	const mayAsk = $derived(session.can('ask'));

	function load() {
		const want = `${mbid}|${artist}`;
		const still = () => want === `${mbid}|${artist}`;
		busy = true;
		error = '';
		// All at once: the hero, the tracks, the similar artists and the ask state.
		void api.get<AlbumPageData>(`/music/albums/${mbid}?artist=${artist}&similar=0`).then((d) => {
			if (!still()) return;
			data = d;
			// A bare album link learns its artist from the full answer: ask the nearby albums now.
			const found = d.artist?.mbid;
			if (!artist && found) void api.get<{ near: PageCredit[] }>(`/music/albums/${mbid}?part=near&artist=${found}`).then((r) => still() && (near = r.near)).catch(() => still() && (near = []));
		}).catch((e) => {
			if (still()) error = message(e);
		}).finally(() => {
			if (still()) busy = false;
		});
		// The quick core and nearby parts need the artist; a bare album link waits for the full answer instead.
		if (artist) void api.get<AlbumCoreData>(`/music/albums/${mbid}?part=core&artist=${artist}`).then((c) => still() && !data && (core = c)).catch(() => {});
		// A bare album link has no artist: skip the artist calls instead of asking for /music/artists/.
		if (!artist) similar = [];
		else void api.get<{ similar: PageArtist[] }>(`/music/artists/${artist}?part=similar&n=14`).then((r) => still() && (similar = r.similar)).catch(() => still() && (similar = []));
		if (artist) void api.get<{ near: PageCredit[] }>(`/music/albums/${mbid}?part=near&artist=${artist}`).then((r) => still() && (near = r.near)).catch(() => still() && (near = []));
		if (artist) void api.get<{ artists: LookupArtist[] }>(`/artists/lookup?term=${encodeURIComponent(`mbid:${artist}`)}`).then((r) => {
			if (still()) lookup = r.artists[0] ?? null;
		}).catch(() => {});
	}

	$effect(() => {
		void mbid;
		void artist;
		core = null;
		data = null;
		similar = null;
		near = null;
		edition = '';
		lookup = null;
		coverIn = false;
		askedHere = false;
		void load();
	});

	onMount(() => {
		if (session.can('manage_library')) {
			void api.get<{ library_folders: LibraryFolder[] }>('/library-folders').then((r) => (folders = r.library_folders)).catch(() => {});
			void api.get<{ pick_rules: PickRulesView[] }>('/pick-rules').then((r) => (rules = r.pick_rules)).catch(() => {});
			void api.get<MusicSettings>('/settings/music').then((r) => (defaults = r)).catch(() => {});
		}
	});

	const hero = $derived<AlbumCoreData | null>(data ?? core);
	// A cover the server has not stored yet is asked for again while the
	// page is open (20 s, 70 s, 150 s) and swaps in once stored; until then
	// the placeholder shows.
	const COVER_RETRY = [20000, 70000, 150000];
	let coverTry = $state(0);
	let coverFailed = $state(false);
	const cover = $derived.by(() => {
		const p = appPicture(hero?.image);
		return p && coverTry ? `${p}${p.includes('?') ? '&' : '?'}fresh=${coverTry}` : p;
	});
	// The hero cover at its slot (the big size only on sharp screens), on
	// its main colour until it arrives; the preload asks the same address.
	const heroCover = $derived(coverFor(cover, 260, undefined, true).src);
	const coverTint = $derived(pictureOf(cover)?.color || null);
	$effect(() => {
		void hero?.mbid;
		coverTry = 0;
	});
	$effect(() => {
		void cover;
		coverFailed = false;
	});
	$effect(() => {
		if (!coverFailed || coverTry >= COVER_RETRY.length) return;
		const t = setTimeout(() => (coverTry += 1), COVER_RETRY[coverTry]);
		return () => clearTimeout(t);
	});
	const titleId = $derived(data?.title_id || lookup?.title_id || 0);
	const asked = $derived(askedHere || !!lookup?.ask?.albums?.some((a) => a.mbid === mbid) || (!!lookup?.ask && !lookup.ask.albums?.length));
	const status = $derived(data ? owned(data, asked) : null);
	// The editions switcher: the picked edition's own list, else the main one.
	const picked = $derived(data?.editions.find((e) => e.mbid === edition) ?? null);
	const list = $derived<PageTrack[]>(picked?.list?.length ? picked.list : (data?.tracks ?? []));
	const discs = $derived.by(() => {
		const m = new Map<number, PageTrack[]>();
		for (const t of list) m.set(t.disc, [...(m.get(t.disc) ?? []), t]);
		return [...m.entries()];
	});
	const haveCount = $derived(list.filter((t) => t.have === 'in_library').length);
	const missingCount = $derived(list.filter((t) => t.have === 'missing').length);
	const meta = $derived.by(() => {
		if (!data) return [];
		const n = data.tracks.length;
		return [kindWord(data), albumYear(data.release_date), data.label, n ? `${n} ${n === 1 ? 'track' : 'tracks'}` : '', length(data.length_ms, true)].filter(Boolean) as string[];
	});

	function act() {
		if (titleId) return void goto(to(data?.part_id ? `/library/${titleId}/albums/${data.part_id}` : `/library/${titleId}`));
		if (!lookup) return void toasts.push('Still reading this artist; try again in a moment.', 'fail');
		if (manage) addDialog?.open(lookup);
		else if (edition) void askEdition();
		else askDialog?.open(lookup, mbid);
	}

	/** A member who picked an edition asks for this album in that edition. */
	async function askEdition() {
		if (asking || !lookup) return;
		asking = true;
		try {
			const r = await api.post<{ ask: Ask; joined: boolean }>('/asks', { kind: 'artist', id: `mbid:${artist}`, albums: [mbid], editions: { [mbid]: edition } });
			onAsked(lookup, r);
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			asking = false;
		}
	}

	function onAsked(_a: LookupArtist, r: { ask: { name: string }; joined: boolean }) {
		askedHere = true;
		toasts.push(r.joined ? 'Someone asked already; you are on the ask too.' : `Asked for ${hero?.name}. You will hear when it is approved.`, 'ok');
		void refreshInbox();
	}

	const cta = $derived(titleId ? (data?.state && data.state !== 'not_included' ? 'Open in Library' : 'Manage in Library') : asked ? 'Asked' : manage ? 'Add artist' : 'Ask for this album');

	const from = $derived.by(() => (void page.url.pathname, cameFrom()));
	function stepBack(e: MouseEvent) {
		if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
		e.preventDefault();
		history.back();
	}
</script>

<svelte:head><title>{hero?.name ?? 'Album'} · HolyArrs</title>{#if heroCover}<link rel="preload" as="image" href={heroCover} />{/if}</svelte:head>

{#snippet albumCard(a: PageAlbum)}
	{@const c = coverFor(appPicture(a.image), 200)}
	<PosterCard title={a.name} href={albumHref(a.mbid, artist)} image={c.src} kind="artist" shape="square" music
		subtitle={[albumYear(a.release_date), kindWord(a)].filter(Boolean).join(' · ')} badges={a.explicit ? ['E'] : undefined} owned={owned(a)} />
{/snippet}

<div class="album">
	{#if error && !data}<LoadError hasData={false} message={plainReason(error).text} onretry={load} {busy} />{/if}
	<PageLoader loading={!hero && !error} label="Loading this album" onretry={load}>
		{#if hero && !(error && !data)}
			<header class="hero" data-hero>
				{#if from?.label}<nav class="crumbs" aria-label="Breadcrumb"><a class="chip" href={from.path} onclick={stepBack} data-testid="album-back"><Icon name={fromIcon(from.path) ?? 'chevronLeft'} size={14} />{from.label}</a></nav>{/if}
				<div class="hero-in">
					<div class="cover" class:in={coverIn} style:background-color={coverTint}>
						{#if cover && !coverFailed}<img src={heroCover} alt={`${hero.name} cover`} width="320" height="320" decoding="async" fetchpriority="high" onload={() => (coverIn = true)} onerror={() => { coverIn = false; coverFailed = true; }} />{:else}<ArtPlaceholder name={hero.name} shape="square" color={coverTint} />{/if}
					</div>
					<div class="words">
						<p class="kind">{kindWord(hero)}</p>
						<h1>{#if data?.explicit}<span class="e big" aria-label="Explicit">E</span>{/if}{hero.name}</h1>
						<a class="by" href={artistHref(hero.artist.mbid)}>
							<img src={appPicture(hero.artist.image)} alt="" width="28" height="28" />{hero.artist.name}
						</a>
						{#if data}<p class="meta">{meta.join(' · ')}</p>{:else}<p class="meta"><span class="sk sk-line meta-sk" aria-hidden="true"></span></p>{/if}
						<div class="actions">
							{#if titleId || manage || mayAsk}
								<Button variant="primary" icon={titleId ? 'library' : asked ? 'check' : 'plus'} disabled={!titleId && (asked || !lookup || asking)} onclick={act} data-testid="album-ask">{cta}</Button>
							{/if}
							{#if status && data}<span class="pill {status.state}" data-testid="album-state">{status.label}{#if data.part_id && data.tracks.length} · {haveCount} of {data.tracks.length} tracks{/if}</span>{/if}
						</div>
					</div>
				</div>
			</header>

			{#if !data}
				<section class="block" aria-busy="true" aria-label="Loading the tracks">
					<div class="tracks">{#each Array.from({ length: 10 }, (_, i) => i) as i (i)}<SkTrackRow height={48} />{/each}</div>
				</section>
				<div class="sq-rail"><SkRail item="var(--music-item)" titleWidth="200px" /></div>
			{:else}
			<section class="block">
								{#if data.incomplete || !data.tracks.length}<p class="note">The tracklist is not known yet.</p>{/if}
				<div class="row-head">
					<h2>Tracks</h2>
					{#if data.part_id && list.length}<p class="count" data-testid="album-have">{haveCount} have · {missingCount} missing</p>{/if}
				</div>
				{#if data.editions.length > 1}
					<div class="chips" role="toolbar" aria-label="Editions" data-testid="album-editions">
						<Chip label="Usual" selected={!edition} onclick={() => (edition = '')} />
						{#each data.editions as ed (ed.mbid)}<Chip label={`${ed.label} · ${ed.tracks}`} selected={edition === ed.mbid} onclick={() => (edition = ed.mbid)} />{/each}
					</div>
					{#if edition && !titleId && mayAsk && !manage}<p class="note">Asking now asks for this edition.</p>{/if}
				{/if}
				{#each discs as [disc, tracks] (disc)}
					{#if discs.length > 1}<h3 class="disc">Disc {disc}</h3>{/if}
					<ol class="tracks">
						{#each tracks as t (t.disc * 1000 + t.number)}
							<li class:have={t.have === 'in_library'}>
								<span class="n">{t.number}</span>
								<span class="t-name">{#if t.explicit}<span class="e" aria-label="Explicit">E</span>{/if}{t.name}</span>
								{#if t.have}
									<span class="t-have {t.have}" title={t.have === 'in_library' ? 'In your Library' : t.have === 'downloading' ? 'Downloading' : 'Missing'}>
										<Icon name={t.have === 'in_library' ? 'check' : t.have === 'downloading' ? 'download' : 'dash'} size={14} />
										<span class="sr-only">{t.have === 'in_library' ? 'In your Library' : t.have === 'downloading' ? 'Downloading' : 'Missing'}</span>
									</span>
								{:else}<span></span>{/if}
								<span class="t-len">{length(t.length_ms)}</span>
							</li>
						{/each}
					</ol>
				{/each}
			</section>

{#if data}
			<section class="block">
				<h2>Details</h2>
				<dl class="kv">
					{#if data.release_date}<div><dt>Released</dt><dd>{data.release_date}</dd></div>{/if}
					<div><dt>Kind</dt><dd>{kindWord(data)}</dd></div>
					{#if data.label}<div><dt>Label</dt><dd>{data.label}</dd></div>{/if}
					{#if list.length}<div><dt>Tracks</dt><dd>{list.length}{data.length_ms ? ` · ${length(data.length_ms, true)}` : ''}</dd></div>{/if}
					{#if data.editions.length}<div><dt>Editions</dt><dd>{data.editions.length}</dd></div>{/if}
				</dl>
			</section>
			{/if}

			{#if data.more_by.length}
				<Shelf title={`More from ${data.artist.name}`} href={artistHref(data.artist.mbid)} item="var(--music-item)" items={data.more_by} key={(a) => a.mbid} placeholder="square" lines={1}>
					{#snippet card(a: PageAlbum)}{@render albumCard(a)}{/snippet}
				</Shelf>
			{/if}

			{/if}

			{#if near?.length}
				<Shelf title="Fans also like" item="var(--music-item)" items={near} key={(a) => a.mbid} placeholder="square" lines={2}>
					{#snippet card(a: PageCredit)}
						{@const c = coverFor(appPicture(a.image), 200)}
						<PosterCard title={a.name} href={albumHref(a.mbid, a.artist_mbid)} image={c.src} kind="artist" shape="square" music subtitle={a.artist_name} badges={a.explicit ? ['E'] : undefined} />
					{/snippet}
				</Shelf>
			{/if}

			{#if similar === null}
				<SkRail kind="person" item="104px" titleWidth="200px" />
			{:else if similar.length}
				<section class="block">
					<h2>If you like {hero.artist.name}</h2>
					<ul class="circles" use:dragScroll>{#each similar as p (p.mbid)}<li><PersonCircle name={p.name} image={appPicture(p.image)} href={artistHref(p.mbid)} size={88} /></li>{/each}</ul>
				</section>
			{/if}

		{/if}
	</PageLoader>
</div>

{#if manage}
	<AddArtistDialog bind:this={addDialog} {folders} {rules} {defaults} onadded={(_a, t) => { toasts.push(`${t.name} is in your Library. Albums and tracks are being filled in.`, 'ok'); void load(); }} />
{:else if mayAsk}
	<AskArtistDialog bind:this={askDialog} onasked={onAsked} />
{/if}

<style>
	.album { --music-item: max(168px, calc((100vw - 96px) / 7.2)); display: flex; flex-direction: column; gap: 32px; padding-bottom: 48px; min-width: 0; }
	.hero { position: relative; margin: 0; color: var(--text); }
	.crumbs { margin-bottom: 8px; }
	.chip { display: inline-flex; align-items: center; gap: 6px; height: 30px; padding: 0 12px 0 10px; border-radius: 999px; background: color-mix(in srgb, var(--text) 8%, transparent); color: var(--text); font: 600 13px/1.2 var(--font-ui); }
	.chip:hover { background: color-mix(in srgb, var(--text) 14%, transparent); }
	.hero-in { display: grid; grid-template-columns: 240px minmax(0, 1fr); gap: 32px; align-items: center; padding: 8px 0 8px; }
	.cover { position: relative; width: 240px; aspect-ratio: 1; display: grid; place-items: center; border-radius: 10px; overflow: hidden; background: var(--surface-1); box-shadow: 0 10px 30px rgb(0 0 0 / .35), 0 2px 6px rgb(0 0 0 / .2); }
	.cover img { width: 100%; height: 100%; object-fit: cover; opacity: 0; transition: opacity 400ms ease-out; }
	.cover.in img { opacity: 1; }
	.words { min-width: 0; display: flex; flex-direction: column; align-items: flex-start; gap: 6px; }
	.kind { margin: 0; color: var(--text-muted); font: 600 12px/1.3 var(--font-ui); text-transform: uppercase; letter-spacing: .08em; }
	h1 { margin: 0; font: 700 26px/1.15 var(--font-display); letter-spacing: -.01em; text-wrap: balance; overflow-wrap: anywhere; }
	.by { display: inline-flex; align-items: center; gap: 8px; color: var(--accent); font: 400 20px/1.2 var(--font-ui); }
	.by img { width: 24px; height: 24px; border-radius: 50%; object-fit: cover; background: var(--surface-1); }
	.by:hover { text-decoration: underline; }
	.meta { margin: 0; color: var(--text-muted); font: 600 12px/1.4 var(--font-ui); font-variant-numeric: tabular-nums; }
	.actions { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; margin-top: 14px; }
	.actions :global(button), .actions :global(a) { min-width: 130px; height: 36px; border-radius: 24px; }
	.pill { padding: 6px 12px; border-radius: 999px; background: color-mix(in srgb, var(--text) 8%, transparent); font: var(--t-footnote); font-weight: 600; font-variant-numeric: tabular-nums; }
	.e { display: inline-grid; place-items: center; width: 15px; height: 15px; margin-right: 8px; border-radius: 3px; background: var(--text-muted); color: var(--bg); font: 700 10px/1 var(--font-ui); vertical-align: 2px; }
	.e.big { width: 26px; height: 26px; font-size: 15px; border-radius: 5px; background: var(--text-muted); color: var(--bg); vertical-align: 6px; }
	.block { display: flex; flex-direction: column; gap: 12px; min-width: 0; }
	h2 { margin: 0; font: var(--t-title2); }
	.disc { margin: 8px 0 0; font: var(--t-headline); color: var(--text-muted); }
	.tracks { margin: 0; padding: 0; list-style: none; max-width: 980px; }
	.tracks li { display: grid; grid-template-columns: 32px minmax(0, 1fr) 28px 56px; align-items: center; gap: 12px; min-height: 48px; padding: 0 10px; border-radius: 8px; }
	.tracks li:nth-child(odd) { background: var(--surface-1, color-mix(in srgb, var(--text) 3%, transparent)); }
	.n { color: var(--text-muted); font: var(--t-subhead); font-variant-numeric: tabular-nums; text-align: right; }
	.t-name { font: var(--t-callout); overflow-wrap: anywhere; }
	.t-have { display: grid; place-items: center; color: var(--text-muted); }
	.t-have.in_library { color: var(--ok); }
	.t-have.downloading { color: var(--accent); }
	.t-len { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; text-align: right; }
	.note { margin: 0; color: var(--text-muted); font: var(--t-subhead); }
	.row-head { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; max-width: 980px; }
	.count { margin: 0; color: var(--text-muted); font: var(--t-subhead); font-variant-numeric: tabular-nums; }
	.chips { display: flex; gap: 8px; flex-wrap: wrap; }
	.kv { margin: 0; display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: 16px; max-width: 980px; }
	.kv dt { color: var(--text-muted); font: var(--t-footnote); text-transform: uppercase; letter-spacing: .05em; }
	.kv dd { margin: 2px 0 0; font: var(--t-callout); }
	.circles { display: flex; gap: 18px; overflow-x: auto; margin: 0; padding: 4px 2px 8px; list-style: none; scrollbar-width: none; }
	.meta-sk { display: inline-block; width: 220px; height: 14px; max-width: 100%; }
	.sq-rail { --poster-ratio: 1; }
	@media (max-width: 767px) {
		.album { --music-item: calc((100cqi - var(--shelf-gap, 12px)) / 2.3); gap: 26px; }
		.hero-in { grid-template-columns: minmax(0, 1fr); justify-items: center; text-align: center; gap: 20px; }
		.cover { width: min(260px, 70vw); }
		.words { align-items: center; width: 100%; }
		.actions { flex-direction: column; align-items: stretch; width: 100%; }
		.actions :global(button), .actions :global(a) { width: 100%; }
		.pill { text-align: center; }
	}
</style>
