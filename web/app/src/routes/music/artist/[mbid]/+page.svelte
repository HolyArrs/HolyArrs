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
	import IconButton from '#lib/ui/IconButton.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/Icon.svelte';
	import AskArtistDialog from '#lib/AskArtistDialog.svelte';
	import AddArtistDialog from '#lib/AddArtistDialog.svelte';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { refreshInbox } from '#lib/wishlist.svelte.ts';
	import { to } from '#lib/paths.ts';
	import { plainReason } from '#lib/plain.ts';
	import { licenseURL, wikipediaURL } from '#lib/wiki-facts.ts';
	import { cameFrom, fromIcon } from '#lib/came-from.svelte.ts';
	import { albumYear } from '#lib/music.ts';
	import { releaseWords } from '#lib/music-trending.ts';
	import { SkRail, SkRowTitle, SkTrackRow } from '#lib/ui/sk/index.ts';
	import { albumHref, appPicture, artistHref, coverFor, haveLine, kindWord, length, owned, shelves, type ArtistCoreData, type ArtistPageData, type PageAlbum, type PageArtist } from '#lib/music-pages.ts';
	import type { Ask, LibraryFolder, LookupArtist, MusicSettings, PickRulesView } from '#lib/types.ts';

	// An artist's page: any MusicBrainz artist, in the Library or not. HolyArrs asks
	// for music; it does not play it, so songs lead to asking for their album.
	const mbid = $derived((page.params.mbid ?? '').toLowerCase());
	// Hero first: the core answer (name, picture, bio) draws the hero; the full
	// answer (albums, songs, links) and the similar artists fill the shelves.
	let core = $state<ArtistCoreData | null>(null);
	let data = $state<ArtistPageData | null>(null);
	let similar = $state<PageArtist[] | null>(null);
	let error = $state('');
	let busy = $state(false);
	let lookup = $state<LookupArtist | null>(null);
	let art = $state<{ logo?: string; backgrounds?: string[]; thumb?: string }>({});
	let following = $state<boolean | null>(null);
	let bioOpen = $state(false);
	let heroIn = $state(false);
	let logoFailed = $state(false);
	let folders = $state<LibraryFolder[]>([]);
	let rules = $state<PickRulesView[]>([]);
	let defaults = $state<MusicSettings | null>(null);
	let askDialog: ReturnType<typeof AskArtistDialog> | undefined = $state();
	let addDialog: ReturnType<typeof AddArtistDialog> | undefined = $state();
	let asked = $state(new Set<string>());
	let follows = $state(new Map<string, boolean>());

	const manage = $derived(session.can('manage_library'));
	const mayAsk = $derived(session.can('ask'));
	const hero = $derived<ArtistCoreData | null>(data ?? core);
	const titleId = $derived(hero?.title_id || lookup?.title_id || 0);

	function load() {
		const want = mbid;
		busy = true;
		error = '';
		let followAsked = false;
		const askFollow = (id?: number) => {
			if (!id || followAsked) return;
			followAsked = true;
			void api.get<{ following: boolean }>(`/titles/${id}/follow`).then((r) => want === mbid && (following = r.following)).catch(() => {});
		};
		// All at once: the hero, the shelves, the similar artists, the artwork and the ask state.
		const coreAsk = api.get<ArtistCoreData>(`/music/artists/${want}?part=core`).then((c) => {
			if (want !== mbid) return;
			if (!data) core = c;
			askFollow(c.title_id);
		});
		const fullAsk = api.get<ArtistPageData>(`/music/artists/${want}?similar=0`).then((d) => {
			if (want !== mbid) return;
			data = d;
			follows = new Map(d.albums.filter((a) => a.upcoming).map((a) => [a.mbid, !!a.following]));
			askFollow(d.title_id);
		});
		void api.get<{ similar: PageArtist[] }>(`/music/artists/${want}?part=similar`).then((r) => want === mbid && (similar = r.similar)).catch(() => want === mbid && (similar = []));
		void api.get<typeof art>(`/extras/artist/${want}`).then((x) => want === mbid && (art = x)).catch(() => {});
		void api.get<{ artists: LookupArtist[] }>(`/artists/lookup?term=${encodeURIComponent(`mbid:${want}`)}`).then((r) => {
			if (want !== mbid) return;
			lookup = r.artists[0] ?? null;
			asked = new Set((lookup?.ask?.albums ?? []).map((a) => a.mbid));
		}).catch(() => {});
		void Promise.allSettled([coreAsk, fullAsk]).then(([c, f]) => {
			if (want !== mbid) return;
			busy = false;
			// The shelves failing shows the error; the core failing alone is quiet (the full answer has the hero too).
			if (f.status === 'rejected') error = message(f.reason);
			else if (c.status === 'rejected' && !data) error = message(c.reason);
		});
	}

	$effect(() => {
		void mbid;
		core = null;
		data = null;
		similar = null;
		lookup = null;
		art = {};
		following = null;
		heroIn = false;
		logoFailed = false;
		bioOpen = false;
		void load();
	});

	onMount(() => {
		if (session.can('manage_library')) {
			void api.get<{ library_folders: LibraryFolder[] }>('/library-folders').then((r) => (folders = r.library_folders)).catch(() => {});
			void api.get<{ pick_rules: PickRulesView[] }>('/pick-rules').then((r) => (rules = r.pick_rules)).catch(() => {});
			void api.get<MusicSettings>('/settings/music').then((r) => (defaults = r)).catch(() => {});
		}
	});

	const background = $derived(art.backgrounds?.[0] ?? null);
	const picture = $derived(appPicture(hero?.image));
	const facts = $derived.by(() => {
		if (!hero) return [];
		const type = hero.type ? hero.type.charAt(0).toUpperCase() + hero.type.slice(1) : '';
		return [type, hero.country, hero.status === 'ended' ? 'Disbanded or retired' : '', (data?.genres ?? []).slice(0, 3).join(', ')].filter(Boolean) as string[];
	});
	const groups = $derived(data ? shelves(data.albums) : []);
	const soon = $derived(data?.albums.filter((a) => a.upcoming) ?? []);

	/** The person's ask, or what they may do: ask (members) or add (managers). */
	function askAlbum(a?: PageAlbum) {
		if (!lookup) return void toasts.push('Still reading this artist; try again in a moment.', 'fail');
		if (titleId) {
			if (a?.part_id) return void goto(to(`/library/${titleId}/albums/${a.part_id}`));
			return void goto(to(`/library/${titleId}`));
		}
		if (manage) addDialog?.open(lookup);
		else askDialog?.open(lookup, a?.mbid);
	}

	function onAsked(_a: LookupArtist, r: { ask: Ask; joined: boolean }) {
		asked = new Set([...asked, ...(r.ask.albums ?? []).map((x) => x.mbid)]);
		lookup = lookup ? { ...lookup, ask: { id: r.ask.id, state: r.ask.state, state_label: r.ask.state_label, mine: true, people: 1, albums: r.ask.albums } } : lookup;
		toasts.push(r.joined ? `Someone asked for ${r.ask.name} already; you are on the ask too.` : `Asked for ${r.ask.name}. You will hear when it is approved.`, 'ok');
		void refreshInbox();
	}

	async function toggleFollow() {
		if (!titleId) return;
		const on = !following;
		following = on;
		try {
			await (on ? api.put(`/titles/${titleId}/follow`, {}) : api.del(`/titles/${titleId}/follow`));
		} catch (e) {
			following = !on;
			toasts.push(message(e), 'fail');
		}
	}

	async function followAlbum(a: PageAlbum) {
		const on = !follows.get(a.mbid);
		follows = new Map([...follows, [a.mbid, on]]);
		try {
			if (on) await api.put(`/metadata/album/${a.mbid}/follow`, { name: a.name, artist: hero?.name ?? '', release_date: a.release_date ?? '' });
			else await api.del(`/metadata/album/${a.mbid}/follow`);
			toasts.push(on ? `You will hear when ${a.name} is out.` : `Stopped following ${a.name}.`, 'ok');
		} catch (e) {
			follows = new Map([...follows, [a.mbid, !on]]);
			toasts.push(message(e), 'fail');
		}
	}

	const albumAsked = (a: PageAlbum) => asked.has(a.mbid) || (!!lookup?.ask && !lookup.ask.albums?.length);
	const cta = $derived(titleId ? 'Open in Library' : lookup?.ask ? 'Asked' : manage ? 'Add artist' : 'Ask for discography');

	const from = $derived.by(() => (void page.url.pathname, cameFrom()));
	function stepBack(e: MouseEvent) {
		if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
		e.preventDefault();
		history.back();
	}
</script>

<svelte:head><title>{hero?.name ?? 'Artist'} · HolyArrs</title></svelte:head>

{#snippet albumCard(a: PageAlbum, eager = false)}
	{@const c = coverFor(appPicture(a.image), 200)}
	<PosterCard title={a.name} href={albumHref(a.mbid, mbid)} image={c.src} srcset={c.srcset} {eager} kind="artist" shape="square" music actionOnHover
		subtitle={[albumYear(a.release_date), a.shelf === 'albums' ? '' : kindWord(a)].filter(Boolean).join(' · ') || undefined}
		badges={a.explicit ? ['E'] : undefined} owned={owned(a, albumAsked(a))}>
		{#snippet action()}
			{#if !titleId && !albumAsked(a) && (manage || mayAsk)}
				<IconButton icon="plus" label={manage ? `Add ${hero?.name}` : `Ask for ${a.name}`} variant="glass" size="lg" aria-haspopup="dialog" onclick={() => askAlbum(a)} />
			{/if}
		{/snippet}
	</PosterCard>
{/snippet}

<div class="artist">
	{#if error && !hero}<LoadError hasData={false} message={plainReason(error).text} onretry={load} {busy} />{/if}
	<PageLoader loading={!hero && !error} label="Loading this artist" onretry={load}>
		{#if hero}
			<header class="hero" data-hero>
				{#if background}
					<img class="art" class:in={heroIn} src={background} alt="" decoding="async" fetchpriority="high" onload={() => (heroIn = true)} onerror={() => (heroIn = false)} />
				{:else if picture}
					<img class="art soft" class:in={heroIn} src={picture} alt="" decoding="async" onload={() => (heroIn = true)} onerror={() => (heroIn = false)} />
				{/if}
				<div class="shade" aria-hidden="true"></div>
				{#if from?.label}<nav class="crumbs" aria-label="Breadcrumb"><a class="chip" href={from.path} onclick={stepBack} data-testid="artist-back"><Icon name={fromIcon(from.path) ?? 'chevronLeft'} size={14} />{from.label}</a></nav>{/if}
				<div class="hero-in">
					{#if picture}<img class="photo" src={picture} alt="" width="160" height="160" />{/if}
					<div class="who">
						{#if art.logo && !logoFailed}
							<h1 class="logo"><img src={art.logo} alt={hero.name} onerror={() => (logoFailed = true)} /></h1>
						{:else}
							<h1>{hero.name}</h1>
						{/if}
						{#if facts.length}<p class="facts">{facts.join(' · ')}</p>{/if}
						{#if data?.total}<p class="have" data-testid="artist-have"><Icon name="music" size={14} />{haveLine(data.have, data.total)}</p>{/if}
						<div class="actions">
							{#if titleId}
								<Button variant="primary" icon="library" href={to(`/library/${titleId}`)}>{cta}</Button>
								{#if following !== null}<Button icon={following ? 'bellOn' : 'bell'} onclick={toggleFollow} aria-pressed={following}>{following ? 'Following' : 'Follow'}</Button>{/if}
							{:else if manage || mayAsk}
								<Button variant="primary" icon={lookup?.ask ? 'check' : 'plus'} disabled={!!lookup?.ask || !lookup} onclick={() => askAlbum()} data-testid="artist-ask">{cta}</Button>
							{/if}
						</div>
						{#if hero.bio}
							<p class="bio" class:open={bioOpen}>{hero.bio.extract}</p>
							<div class="bio-foot">
								<button class="more" type="button" onclick={() => (bioOpen = !bioOpen)} aria-expanded={bioOpen}>{bioOpen ? 'Less' : 'More'}</button>
								<p class="credit" data-testid="wiki-credit">From <a href={wikipediaURL(hero.bio.url) || 'https://en.wikipedia.org/'} target="_blank" rel="noopener noreferrer">Wikipedia</a>, <a href={licenseURL(hero.bio.license_url)} target="_blank" rel="noopener noreferrer license">CC BY-SA</a></p>
							</div>
						{:else if !data}
							<span class="bio-sk" aria-hidden="true"><span class="sk sk-line"></span><span class="sk sk-line"></span><span class="sk sk-line short"></span></span>
						{/if}
					</div>
				</div>
			</header>

			{#if !data}
				{#if !error}
					<section class="lead" aria-busy="true" aria-label="Loading albums and songs">
						<div class="latest"><SkRowTitle width="140px" /><div class="latest-card sk-latest"><span class="sk sq"></span><span class="sk-lines"><span class="sk sk-line"></span><span class="sk sk-line"></span><span class="sk sk-line short"></span></span></div></div>
						<div class="top"><SkRowTitle width="120px" /><div class="sk-songs">{#each Array.from({ length: 8 }, (_, i) => i) as i (i)}<SkTrackRow height={58} />{/each}</div></div>
					</section>
					<div class="sq-rail"><SkRail item="var(--music-item)" titleWidth="110px" /></div>
					<div class="sq-rail"><SkRail item="var(--music-item)" titleWidth="160px" /></div>
				{:else}
					<LoadError hasData={true} message={plainReason(error).text} onretry={load} {busy} />
				{/if}
			{:else if data.latest || data.top.length}
				<section class="lead">
					{#if data.latest}
						{@const l = data.latest}
						<div class="latest">
							<h2>Latest release</h2>
							<a class="latest-card" href={albumHref(l.mbid, mbid)}>
								<img src={coverFor(appPicture(l.image), 200).src} srcset={coverFor(appPicture(l.image), 200).srcset} sizes="200px" alt="" width="200" height="200" loading="lazy" decoding="async" />
								<span class="l-meta">
									<span class="l-date">{l.release_date ? new Date(`${l.release_date.padEnd(10, '-01').slice(0, 10)}T00:00:00Z`).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: l.release_date.length === 10 ? 'numeric' : undefined, timeZone: 'UTC' }) : ''}</span>
									<span class="l-name">{#if l.explicit}<span class="e" aria-label="Explicit">E</span>{/if}{l.name}</span>
									<span class="l-kind">{kindWord(l)}</span>
									{#if owned(l, albumAsked(l))}{@const o = owned(l, albumAsked(l))}<span class="pill {o?.state}">{o?.label}</span>{/if}
								</span>
							</a>
						</div>
					{/if}
					{#if data.top.length}
						<div class="top">
							<h2>Top songs</h2>
							<ol class="songs">
								{#each data.top.slice(0, 8) as s, i (s.title + i)}
									<li>
										<span class="n">{i + 1}</span>
										{#if s.image}<img src={coverFor(appPicture(s.image), 40).src} alt="" width="40" height="40" loading="lazy" decoding="async" />{:else}<span class="ph"></span>{/if}
										<span class="s-words">
											<span class="s-name">{#if s.explicit}<span class="e" aria-label="Explicit">E</span>{/if}{s.title}</span>
											{#if s.album_mbid}<a class="s-album" href={albumHref(s.album_mbid, mbid)}>{s.album}</a>{:else}<span class="s-album">{s.album}</span>{/if}
										</span>
										<span class="s-len">{length(s.length_ms)}</span>
										{#if s.album_mbid && !titleId && (manage || mayAsk)}
											{@const al = data.albums.find((x) => x.mbid === s.album_mbid)}
											{#if al && !albumAsked(al)}
												<IconButton icon="plus" size="sm" label={`Ask for the album ${s.album}`} onclick={() => askAlbum(al)} />
											{:else}
												<span class="s-done" title="Asked"><Icon name="check" size={14} /></span>
											{/if}
										{/if}
									</li>
								{/each}
							</ol>
						</div>
					{/if}
				</section>
			{/if}

			{#if data}
			{#each groups as g, gi (g.shelf)}
				<Shelf title={g.title} item="var(--music-item)" items={g.albums} key={(a) => a.mbid} placeholder="square" lines={1}>
					{#snippet card(a: PageAlbum, i: number)}{@render albumCard(a, gi === 0 && i < 6)}{/snippet}
				</Shelf>
			{/each}

			{#if soon.length}
				<section class="block">
					<h2>Coming soon</h2>
					<ul class="soon">
						{#each soon as a (a.mbid)}
							<li>
								<a href={albumHref(a.mbid, mbid)}><img src={coverFor(appPicture(a.image), 72).src} alt="" width="72" height="72" loading="lazy" decoding="async" /></a>
								<span class="soon-words"><a class="s-name" href={albumHref(a.mbid, mbid)}>{a.name}</a><span class="s-album">{kindWord(a)} · {releaseWords(a.release_date ?? '')}</span></span>
								{#if !data.kid}<IconButton icon={follows.get(a.mbid) ? 'bellOn' : 'bell'} label={follows.get(a.mbid) ? `Stop following ${a.name}` : `Tell me when ${a.name} is out`} aria-pressed={!!follows.get(a.mbid)} onclick={() => followAlbum(a)} />{/if}
							</li>
						{/each}
					</ul>
				</section>
			{/if}

			{#if data.related.length}
				<section class="block">
					<h2>Bands and collaborators</h2>
					<ul class="circles">{#each data.related as p (p.mbid)}<li><PersonCircle name={p.name} role={p.note} image={appPicture(p.image)} href={artistHref(p.mbid)} size={112} /></li>{/each}</ul>
				</section>
			{/if}

			{/if}

			{#if similar === null}
				<SkRail kind="person" item="128px" titleWidth="150px" />
			{:else if similar.length}
				<section class="block">
					<h2>Similar artists</h2>
					<ul class="circles">{#each similar as p (p.mbid)}<li><PersonCircle name={p.name} image={appPicture(p.image)} href={artistHref(p.mbid)} size={112} /></li>{/each}</ul>
				</section>
			{/if}

			<p class="attribution">Albums from MusicBrainz; popularity and explicit marks from Deezer; artwork from fanart.tv and the Cover Art Archive.</p>
		{/if}
	</PageLoader>
</div>

{#if manage}
	<AddArtistDialog bind:this={addDialog} {folders} {rules} {defaults} onadded={(_a, t) => { toasts.push(`${t.name} is in your Library. Albums and tracks are being filled in.`, 'ok'); void load(); }} />
{:else if mayAsk}
	<AskArtistDialog bind:this={askDialog} onasked={onAsked} />
{/if}

<style>
	.artist { --music-item: max(168px, calc((100vw - 96px) / 7.2)); display: flex; flex-direction: column; gap: 36px; padding-bottom: 48px; min-width: 0; }
	/* The hero stays dark in both themes, like the title and person pages. */
	.hero { position: relative; isolation: isolate; min-height: clamp(400px, 42vw, 600px); display: flex; align-items: flex-end; overflow: hidden; margin: calc(var(--s-6) * -1) calc(var(--gutter) * -1) 0; background: var(--on-art-ink); color: var(--on-art); }
	.art { position: absolute; inset: 0; z-index: -2; width: 100%; height: 100%; object-fit: cover; object-position: center 22%; opacity: 0; transition: opacity 400ms ease-out; }
	.art.in { opacity: 1; }
	/* Only a square photo: blown up and blurred into a wash, never stretched sharp. */
	.art.soft { filter: blur(48px) saturate(140%); transform: scale(1.25); }
	.art.soft.in { opacity: .7; }
	.shade { position: absolute; inset: 0; z-index: -1; background: linear-gradient(to top, var(--on-art-scrim) 0%, var(--on-art-scrim-mid) 40%, var(--on-art-scrim-none) 75%), linear-gradient(to right, var(--on-art-scrim-mid) 0%, var(--on-art-scrim-none) 62%); }
	.hero-in { width: 100%; display: grid; grid-template-columns: auto minmax(0, 760px); gap: clamp(18px, 3vw, 36px); align-items: end; padding: clamp(20px, 4vw, 48px); }
	.photo { width: clamp(112px, 13vw, 184px); aspect-ratio: 1; border-radius: 50%; object-fit: cover; box-shadow: 0 18px 48px rgb(0 0 0 / .5), 0 0 0 1px rgb(255 255 255 / .12); }
	.crumbs { position: absolute; top: clamp(14px, 2vw, 22px); left: clamp(20px, 4vw, 48px); z-index: 1; }
	.chip { display: inline-flex; align-items: center; gap: 6px; height: 30px; padding: 0 12px 0 10px; border-radius: 999px; background: rgb(255 255 255 / 14%); border: 1px solid rgb(255 255 255 / 18%); backdrop-filter: blur(18px) saturate(160%); -webkit-backdrop-filter: blur(18px) saturate(160%); color: #fff; font: 600 13px/1.2 var(--font-ui); }
	.chip:hover { background: rgb(255 255 255 / 24%); }
	.who { min-width: 0; display: flex; flex-direction: column; gap: 10px; }
	h1 { margin: 0; font: var(--t-large-title); font-family: var(--font-display); font-size: clamp(34px, 5.4vw, 68px); letter-spacing: var(--t-large-title-track); text-wrap: balance; }
	h1.logo img { display: block; max-width: min(460px, 100%); max-height: 140px; object-fit: contain; object-position: left bottom; filter: drop-shadow(0 4px 18px rgb(0 0 0 / .45)); }
	.facts { margin: 0; color: var(--on-art-muted); font: var(--t-subhead); }
	.have { display: inline-flex; align-items: center; gap: 6px; align-self: flex-start; margin: 0; padding: 5px 12px; border-radius: 999px; background: rgb(255 255 255 / .14); backdrop-filter: blur(12px); font: var(--t-footnote); font-weight: 600; font-variant-numeric: tabular-nums; }
	.actions { display: flex; flex-wrap: wrap; gap: 10px; margin-top: 4px; }
	.bio { margin: 4px 0 0; max-width: 72ch; font: var(--t-callout); line-height: 1.55; display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
	.bio.open { display: block; }
	.bio-foot { display: flex; align-items: baseline; gap: 14px; flex-wrap: wrap; }
	.more { padding: 0; border: 0; background: none; color: var(--on-art); font: var(--t-subhead); font-weight: 600; cursor: pointer; text-decoration: underline; }
	.credit { margin: 0; font: var(--t-caption, 12px/1.4 var(--font-ui)); opacity: .7; }
	.credit a { color: inherit; text-decoration: underline; }
	h2 { margin: 0; font: var(--t-title2); }
	.lead { display: grid; grid-template-columns: minmax(260px, 340px) minmax(0, 1fr); gap: clamp(20px, 4vw, 56px); align-items: start; }
	.latest, .top { display: flex; flex-direction: column; gap: 14px; min-width: 0; }
	.latest-card { display: grid; grid-template-columns: 148px minmax(0, 1fr); gap: 16px; align-items: center; color: var(--text); border-radius: var(--r-card); }
	.latest-card img { width: 148px; height: 148px; border-radius: var(--r-card); object-fit: cover; box-shadow: var(--shadow-card); transition: transform var(--dur) var(--ease-out); background: var(--surface-2); }
	.latest-card:hover img { transform: translateY(var(--lift)); }
	.l-meta { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
	.l-date { color: var(--text-muted); font: var(--t-footnote); text-transform: uppercase; letter-spacing: .04em; }
	.l-name { font: var(--t-headline); overflow-wrap: anywhere; }
	.l-kind { color: var(--text-muted); font: var(--t-subhead); }
	.pill { align-self: flex-start; margin-top: 4px; padding: 3px 10px; border-radius: 999px; background: var(--surface-2); font: var(--t-footnote); font-weight: 600; }
	.pill.available { color: var(--ok); }
	.pill.downloading { color: var(--accent); }
	.songs { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 28px; margin: 0; padding: 0; list-style: none; }
	.songs li { display: grid; grid-template-columns: 22px 40px minmax(0, 1fr) auto 32px; align-items: center; gap: 12px; min-height: 58px; border-bottom: 1px solid var(--line); }
	.songs img, .songs .ph { width: 40px; height: 40px; border-radius: 6px; object-fit: cover; background: var(--surface-2); }
	.n { color: var(--text-muted); font: var(--t-subhead); font-variant-numeric: tabular-nums; text-align: right; }
	.s-words, .soon-words { display: flex; flex-direction: column; min-width: 0; }
	.s-name { font: var(--t-callout); font-weight: 600; color: var(--text); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.s-album { color: var(--text-muted); font: var(--t-footnote); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	a.s-album:hover { text-decoration: underline; }
	.s-len { color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.s-done { display: grid; place-items: center; color: var(--text-muted); }
	.e { display: inline-grid; place-items: center; width: 15px; height: 15px; margin-right: 6px; border-radius: 3px; background: var(--text-muted); color: var(--bg); font: 700 10px/1 var(--font-ui); vertical-align: 1px; }
	.block { display: flex; flex-direction: column; gap: 14px; min-width: 0; }
	.soon { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 320px), 1fr)); gap: 12px; margin: 0; padding: 0; list-style: none; }
	.soon li { display: grid; grid-template-columns: 72px minmax(0, 1fr) auto; gap: 14px; align-items: center; padding: 10px; border-radius: var(--r-card); background: var(--surface-1, var(--surface-2)); }
	.soon img { width: 72px; height: 72px; border-radius: 8px; object-fit: cover; background: var(--surface-2); }
	.circles { display: flex; gap: 18px; overflow-x: auto; margin: 0; padding: 4px 2px 8px; list-style: none; scrollbar-width: none; }
	.bio-sk { display: flex; flex-direction: column; gap: 8px; max-width: 72ch; margin-top: 4px; }
	.bio-sk .sk-line { height: 14px; width: 100%; }
	.bio-sk .short { width: 55%; }
	.sk-latest .sq { width: 148px; height: 148px; border-radius: var(--r-card); }
	.sk-lines { display: flex; flex-direction: column; gap: 8px; }
	.sk-lines .sk-line { height: 13px; width: 80%; }
	.sk-lines .short { width: 45%; }
	.sk-songs { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 28px; }
	.sq-rail { --poster-ratio: 1; }
	.attribution { margin: 0; color: var(--text-muted); font: var(--t-footnote); }
	@media (max-width: 1100px) { .songs, .sk-songs { grid-template-columns: minmax(0, 1fr); } }
	@media (max-width: 767px) {
		.artist { --music-item: calc((100cqi - var(--shelf-gap, 12px)) / 2.3); gap: 28px; }
		.hero-in { grid-template-columns: minmax(0, 1fr); }
		.photo { width: 96px; }
		.lead { grid-template-columns: minmax(0, 1fr); }
	}
</style>
