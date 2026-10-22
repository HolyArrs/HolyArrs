<script lang="ts">
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
	import Icon from '#lib/Icon.svelte';
	import AskArtistDialog from '#lib/AskArtistDialog.svelte';
	import AddArtistDialog from '#lib/AddArtistDialog.svelte';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { refreshInbox } from '#lib/wishlist.svelte.ts';
	import { to } from '#lib/paths.ts';
	import { plainReason } from '#lib/plain.ts';
	import { cameFrom, fromIcon } from '#lib/came-from.svelte.ts';
	import { albumYear } from '#lib/music.ts';
	import { SkRail, SkTrackRow } from '#lib/ui/sk/index.ts';
	import { albumHref, appPicture, artistHref, coverFor, kindWord, length, owned, type AlbumCoreData, type AlbumPageData, type PageAlbum, type PageArtist } from '#lib/music-pages.ts';
	import type { LibraryFolder, LookupArtist, MusicSettings, PickRulesView } from '#lib/types.ts';

	// An album's page: the cover large over its own blurred colours, the tracklist
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

	const manage = $derived(session.can('manage_library'));
	const mayAsk = $derived(session.can('ask'));

	function load() {
		const want = `${mbid}|${artist}`;
		const still = () => want === `${mbid}|${artist}`;
		busy = true;
		error = '';
		// All at once: the hero, the tracks, the similar artists and the ask state.
		void api.get<AlbumCoreData>(`/music/albums/${mbid}?part=core&artist=${artist}`).then((c) => still() && !data && (core = c)).catch(() => {});
		void api.get<{ similar: PageArtist[] }>(`/music/artists/${artist}?part=similar&n=14`).then((r) => still() && (similar = r.similar)).catch(() => still() && (similar = []));
		void api.get<{ artists: LookupArtist[] }>(`/artists/lookup?term=${encodeURIComponent(`mbid:${artist}`)}`).then((r) => {
			if (still()) lookup = r.artists[0] ?? null;
		}).catch(() => {});
		void api.get<AlbumPageData>(`/music/albums/${mbid}?artist=${artist}&similar=0`).then((d) => {
			if (still()) data = d;
		}).catch((e) => {
			if (still()) error = message(e);
		}).finally(() => {
			if (still()) busy = false;
		});
	}

	$effect(() => {
		void mbid;
		void artist;
		core = null;
		data = null;
		similar = null;
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
	const cover = $derived(appPicture(hero?.image));
	const titleId = $derived(data?.title_id || lookup?.title_id || 0);
	const asked = $derived(askedHere || !!lookup?.ask?.albums?.some((a) => a.mbid === mbid) || (!!lookup?.ask && !lookup.ask.albums?.length));
	const status = $derived(data ? owned(data, asked) : null);
	const discs = $derived.by(() => {
		const m = new Map<number, AlbumPageData['tracks']>();
		for (const t of data?.tracks ?? []) m.set(t.disc, [...(m.get(t.disc) ?? []), t]);
		return [...m.entries()];
	});
	const haveCount = $derived(data?.tracks.filter((t) => t.have === 'in_library').length ?? 0);
	const meta = $derived.by(() => {
		if (!data) return [];
		const n = data.tracks.length;
		return [kindWord(data), albumYear(data.release_date), data.label, n ? `${n} ${n === 1 ? 'track' : 'tracks'}` : '', length(data.length_ms, true)].filter(Boolean) as string[];
	});

	function act() {
		if (titleId) return void goto(to(data?.part_id ? `/library/${titleId}/albums/${data.part_id}` : `/library/${titleId}`));
		if (!lookup) return void toasts.push('Still reading this artist; try again in a moment.', 'fail');
		if (manage) addDialog?.open(lookup);
		else askDialog?.open(lookup, mbid);
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

<svelte:head><title>{hero?.name ?? 'Album'} · HolyArrs</title>{#if cover}<link rel="preload" as="image" href={cover} />{/if}</svelte:head>

{#snippet albumCard(a: PageAlbum)}
	{@const c = coverFor(appPicture(a.image), 200)}
	<PosterCard title={a.name} href={albumHref(a.mbid, artist)} image={c.src} srcset={c.srcset} sizes="200px" kind="artist" shape="square" music
		subtitle={[albumYear(a.release_date), kindWord(a)].filter(Boolean).join(' · ')} badges={a.explicit ? ['E'] : undefined} owned={owned(a)} />
{/snippet}

<div class="album">
	{#if error && !data}<LoadError hasData={false} message={plainReason(error).text} onretry={load} {busy} />{/if}
	<PageLoader loading={!hero && !error} label="Loading this album" onretry={load}>
		{#if hero && !(error && !data)}
			<header class="hero" data-hero>
				{#if cover}<img class="wash" src={cover} alt="" aria-hidden="true" />{/if}
				<div class="shade" aria-hidden="true"></div>
				{#if from?.label}<nav class="crumbs" aria-label="Breadcrumb"><a class="chip" href={from.path} onclick={stepBack} data-testid="album-back"><Icon name={fromIcon(from.path) ?? 'chevronLeft'} size={14} />{from.label}</a></nav>{/if}
				<div class="hero-in">
					<div class="cover" class:in={coverIn}>
						{#if cover}<img src={coverFor(cover, 320).src} srcset={coverFor(cover, 320).srcset} sizes="320px" alt={`${hero.name} cover`} width="320" height="320" decoding="async" fetchpriority="high" onload={() => (coverIn = true)} />{:else}<Icon name="music" size={48} />{/if}
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
								<Button variant="primary" icon={titleId ? 'library' : asked ? 'check' : 'plus'} disabled={!titleId && (asked || !lookup)} onclick={act} data-testid="album-ask">{cta}</Button>
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
				<h2 class="sr-only">Tracks</h2>
				{#if data.incomplete || !data.tracks.length}<p class="note">The tracklist is not known yet.</p>{/if}
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

			{#if data.editions.length > 1}
				<section class="block">
					<h2>Other editions</h2>
					<ul class="editions">{#each data.editions as ed (ed.mbid)}<li title={ed.date ?? ''}>{ed.label}<span>{ed.tracks} tracks{ed.date ? ` · ${ed.date.slice(0, 4)}` : ''}</span></li>{/each}</ul>
				</section>
			{/if}

			{#if data.more_by.length}
				<Shelf title={`More from ${data.artist.name}`} href={artistHref(data.artist.mbid)} item="var(--music-item)" items={data.more_by} key={(a) => a.mbid} placeholder="square" lines={1}>
					{#snippet card(a: PageAlbum)}{@render albumCard(a)}{/snippet}
				</Shelf>
			{/if}

			{/if}

			{#if similar === null}
				<SkRail kind="person" item="128px" titleWidth="200px" />
			{:else if similar.length}
				<section class="block">
					<h2>If you like {hero.artist.name}</h2>
					<ul class="circles">{#each similar as p (p.mbid)}<li><PersonCircle name={p.name} image={appPicture(p.image)} href={artistHref(p.mbid)} size={112} /></li>{/each}</ul>
				</section>
			{/if}

			<p class="attribution">Tracks and editions from MusicBrainz; label and explicit marks from Deezer; cover from the Cover Art Archive.</p>
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
	.hero { position: relative; isolation: isolate; overflow: hidden; margin: calc(var(--s-6) * -1) calc(var(--gutter) * -1) 0; background: var(--on-art-ink); color: var(--on-art); }
	/* The cover's own colours, blurred far past any detail, tint the hero. */
	.wash { position: absolute; inset: -10%; z-index: -2; width: 120%; height: 120%; object-fit: cover; filter: blur(64px) saturate(160%); opacity: .75; }
	.shade { position: absolute; inset: 0; z-index: -1; background: linear-gradient(to top, var(--on-art-scrim) 0%, var(--on-art-scrim-mid) 55%, rgb(0 0 0 / .25) 100%); }
	.crumbs { position: absolute; top: clamp(14px, 2vw, 22px); left: clamp(20px, 4vw, 48px); z-index: 1; }
	.chip { display: inline-flex; align-items: center; gap: 6px; height: 30px; padding: 0 12px 0 10px; border-radius: 999px; background: rgb(255 255 255 / 14%); border: 1px solid rgb(255 255 255 / 18%); backdrop-filter: blur(18px) saturate(160%); -webkit-backdrop-filter: blur(18px) saturate(160%); color: #fff; font: 600 13px/1.2 var(--font-ui); }
	.chip:hover { background: rgb(255 255 255 / 24%); }
	.hero-in { display: grid; grid-template-columns: clamp(200px, 24vw, 320px) minmax(0, 1fr); gap: clamp(20px, 3.5vw, 44px); align-items: end; padding: clamp(64px, 7vw, 96px) clamp(20px, 4vw, 48px) clamp(24px, 4vw, 44px); }
	.cover { aspect-ratio: 1; display: grid; place-items: center; border-radius: 10px; overflow: hidden; background: rgb(255 255 255 / .08); box-shadow: 0 24px 64px rgb(0 0 0 / .55), 0 0 0 1px rgb(255 255 255 / .1); }
	.cover img { width: 100%; height: 100%; object-fit: cover; opacity: 0; transition: opacity 400ms ease-out; }
	.cover.in img { opacity: 1; }
	.words { min-width: 0; display: flex; flex-direction: column; gap: 10px; }
	.kind { margin: 0; color: var(--on-art-muted); font: var(--t-footnote); font-weight: 600; text-transform: uppercase; letter-spacing: .08em; }
	h1 { margin: 0; font: var(--t-large-title); font-family: var(--font-display); font-size: clamp(30px, 4.6vw, 60px); letter-spacing: var(--t-large-title-track); text-wrap: balance; overflow-wrap: anywhere; }
	.by { display: inline-flex; align-items: center; gap: 10px; align-self: flex-start; color: var(--on-art); font: var(--t-headline); }
	.by img { width: 28px; height: 28px; border-radius: 50%; object-fit: cover; background: rgb(255 255 255 / .12); }
	.by:hover { text-decoration: underline; }
	.meta { margin: 0; color: var(--on-art-muted); font: var(--t-subhead); font-variant-numeric: tabular-nums; }
	.actions { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; margin-top: 6px; }
	.pill { padding: 6px 12px; border-radius: 999px; background: rgb(255 255 255 / .14); backdrop-filter: blur(12px); font: var(--t-footnote); font-weight: 600; font-variant-numeric: tabular-nums; }
	.e { display: inline-grid; place-items: center; width: 15px; height: 15px; margin-right: 8px; border-radius: 3px; background: var(--text-muted); color: var(--bg); font: 700 10px/1 var(--font-ui); vertical-align: 2px; }
	.e.big { width: 26px; height: 26px; font-size: 15px; border-radius: 5px; background: rgb(255 255 255 / .7); color: #000; vertical-align: 8px; }
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
	.editions { display: flex; flex-wrap: wrap; gap: 8px; margin: 0; padding: 0; list-style: none; }
	.editions li { display: inline-flex; align-items: baseline; gap: 8px; padding: 7px 14px; border-radius: 999px; border: 1px solid var(--line-strong); font: var(--t-subhead); font-weight: 600; }
	.editions span { color: var(--text-muted); font: var(--t-footnote); }
	.circles { display: flex; gap: 18px; overflow-x: auto; margin: 0; padding: 4px 2px 8px; list-style: none; scrollbar-width: none; }
	.meta-sk { display: inline-block; width: 220px; height: 14px; max-width: 100%; }
	.sq-rail { --poster-ratio: 1; }
	.attribution { margin: 0; color: var(--text-muted); font: var(--t-footnote); }
	@media (max-width: 767px) {
		.album { --music-item: calc((100cqi - var(--shelf-gap, 12px)) / 2.3); gap: 26px; }
		.hero-in { grid-template-columns: minmax(0, 1fr); justify-items: center; text-align: center; }
		.cover { width: min(260px, 70vw); }
		.words { align-items: center; }
		.by { align-self: center; }
		.actions { justify-content: center; }
	}
</style>
