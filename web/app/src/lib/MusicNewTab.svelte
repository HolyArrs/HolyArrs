<script lang="ts">
	// New & Popular → Music: a shelf of large editorial cards for the week's
	// biggest new albums, a compact list of fresh tracks, then album rows.
	// Every action asks for (or adds) music; nothing here plays.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api, message } from './api.ts';
	import { toasts } from './toasts.svelte.ts';
	import { refreshInbox } from './wishlist.svelte.ts';
	import { musicCardHref, musicImage } from './music-trending.ts';
	import { exploreLinks, eyebrow, heroNote, newMusicPath, rowNote, type NewMusicAnswer, type NewMusicCard } from './music-new.ts';
	import type { Ask, LibraryFolder, LookupArtist, MusicSettings, PickRulesView, TitleView } from './types.ts';
	import Shelf from './ui/Shelf.svelte';
	import PosterCard from './ui/PosterCard.svelte';
	import IconButton from './ui/IconButton.svelte';
	import Chip from './ui/Chip.svelte';
	import EmptyState from './ui/EmptyState.svelte';
	import AddArtistDialog from './AddArtistDialog.svelte';
	import AskArtistDialog from './AskArtistDialog.svelte';

	let { manage, mayAsk, folders, rules }: { manage: boolean; mayAsk: boolean; folders: LibraryFolder[]; rules: PickRulesView[] } = $props();

	let answer = $state<NewMusicAnswer | null>(null);
	let failed = $state(false);
	let busy = $state('');
	let addedHere = $state(new Map<string, number>());
	let follows = $state(new Map<string, boolean>());
	let defaults = $state<MusicSettings | null>(null);
	let addArtist: ReturnType<typeof AddArtistDialog> | undefined = $state();
	let askArtist: ReturnType<typeof AskArtistDialog> | undefined = $state();
	const gone = new AbortController();
	let poll: ReturnType<typeof setTimeout> | undefined;

	async function load(tries = 0) {
		try {
			const a = await api.get<NewMusicAnswer>(newMusicPath, gone.signal);
			answer = a;
			failed = false;
			if (a.building && tries < 30) poll = setTimeout(() => void load(tries + 1), 3000);
		} catch {
			if (!gone.signal.aborted) failed = true;
		}
	}

	onMount(() => {
		void load();
		if (manage) api.get<MusicSettings>('/settings/music').then((r) => (defaults = r)).catch(() => {});
		return () => {
			clearTimeout(poll);
			gone.abort();
		};
	});

	function live(c: NewMusicCard): NewMusicCard {
		const id = c.artist_mbid ? addedHere.get(c.artist_mbid) : undefined;
		const f = follows.get(c.mbid);
		return { ...c, title_id: c.title_id ?? id, following: f ?? c.following };
	}
	const canAct = (c: NewMusicCard) => !!c.artist_mbid && (manage || mayAsk) && !c.upcoming;

	/** quick finds the artist and opens the add sheet, or the ask sheet with this album picked. */
	async function quick(c: NewMusicCard) {
		const artist = c.artist || c.name;
		if (busy || !c.artist_mbid) return;
		busy = c.key;
		try {
			const r = await api.get<{ artists: LookupArtist[] }>(`/artists/lookup?term=${encodeURIComponent(`mbid:${c.artist_mbid}`)}`);
			const a = r.artists?.find((x) => x.mbid === c.artist_mbid);
			if (!a) return void toasts.push(`Could not find ${artist}. Try Search.`, 'fail');
			if (a.title_id && manage) addedHere = new Map([...addedHere, [a.mbid, a.title_id]]);
			if (manage && !a.title_id) addArtist?.open(a);
			else if (manage) toasts.push(`${a.name} is in your Library; ${c.name} comes with its next refresh.`, 'ok');
			else askArtist?.open(a, c.mbid);
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = '';
		}
	}

	async function follow(c: NewMusicCard) {
		const on = !live(c).following;
		follows = new Map([...follows, [c.mbid, on]]);
		try {
			const path = `/metadata/album/${c.mbid}/follow`;
			if (on) await api.put(path, { name: c.name, artist: c.artist ?? '', release_date: c.date ?? '' });
			else await api.del(path);
			toasts.push(on ? `You will hear when ${c.name} is out.` : `Stopped following ${c.name}.`, 'ok');
		} catch (e) {
			follows = new Map([...follows, [c.mbid, !on]]);
			toasts.push(message(e), 'fail');
		}
	}

	function added(_a: LookupArtist, t: TitleView) {
		addedHere = new Map([...addedHere, [_a.mbid, t.id]]);
		toasts.push(`${t.name} is in your Library. Albums, Pressings and tracks are being filled in.`, 'ok');
	}
	function asked(_a: LookupArtist, r: { ask: Ask; joined: boolean }) {
		if (r.ask.state === 'approved') toasts.push(`${r.ask.name} is approved; HolyArrs keeps it up to date.`, 'ok');
		else if (r.ask.state === 'asked' && r.joined) toasts.push(`Someone asked for ${r.ask.name} already; you are on the ask too.`, 'ok');
		else toasts.push(`Asked for ${r.ask.name}. You will hear when it is approved.`, 'ok');
		void refreshInbox();
	}

	// The hero shelf: paddles page one screen at a time; the left one shows once scrolled.
	let shelf = $state<HTMLElement>();
	let atStart = $state(true);
	let atEnd = $state(false);
	function edges() {
		if (!shelf) return;
		atStart = shelf.scrollLeft < 4;
		atEnd = shelf.scrollLeft + shelf.clientWidth >= shelf.scrollWidth - 4;
	}
	function page(dir: 1 | -1) {
		if (!shelf) return;
		const still = matchMedia('(prefers-reduced-motion: reduce)').matches;
		shelf.scrollBy({ left: dir * shelf.clientWidth, behavior: still ? 'auto' : 'smooth' });
	}
	$effect(() => {
		if (answer?.hero.length) requestAnimationFrame(edges);
	});

	const tracks = $derived(answer?.rows.find((r) => r.type === 'np_music_tracks'));
	const rows = $derived((answer?.rows ?? []).filter((r) => r.type !== 'np_music_tracks'));
	const links = $derived(exploreLinks(answer?.genres ?? []));
	const actLabel = (c: NewMusicCard) => (manage ? `Add ${c.artist || c.name}` : `Ask for ${c.name}`);
</script>

{#if answer?.kid}
	<EmptyState icon="music" title="Music is not shown on this account" text="Music has no age ratings, so it stays off for accounts with an age limit." />
{:else if failed && !answer}
	<EmptyState icon="music" title="Could not load new music" text="The music lists did not answer. Try again shortly." />
{:else if !answer || (answer.building && !answer.rows.length)}
	<div class="loading" aria-busy="true" aria-label="Loading new music">
		<div class="hero-sk">{#each [0, 1, 2] as i (i)}<span></span>{/each}</div>
		{#if answer?.building}<p class="hint">New releases are being gathered. This takes a minute the first time.</p>{/if}
	</div>
{:else if !answer.rows.length && !answer.hero.length}
	<EmptyState icon="music" title="No new music yet" text="New releases fill in within a few minutes." />
{:else}
	<section class="music-new" data-testid="new-music">
		{#if answer.hero.length}
			<div class="hero-wrap">
				<div class="hero" bind:this={shelf} onscroll={edges} role="list" aria-label="This week's biggest new releases" data-testid="new-music-hero">
					{#each answer.hero as raw (raw.key)}
						{@const c = live(raw)}
						<article class="h-card" role="listitem">
							<a class="h-link" href={musicCardHref(c)}>
								<span class="eyebrow">{eyebrow(c)}</span>
								<span class="h-title">{c.name}</span>
								<span class="h-artist">{c.artist}</span>
								<span class="art">
									<img class="wide" src={musicImage({ image: c.artist_image || c.image })} alt="" loading="lazy" onerror={(e) => ((e.currentTarget as HTMLImageElement).style.visibility = 'hidden')} />
									<span class="shade"></span>
									<span class="note">{heroNote(c)}</span>
									{#if c.image}<img class="cover" src={musicImage(c)} alt="" loading="lazy" onerror={(e) => ((e.currentTarget as HTMLImageElement).hidden = true)} />{/if}
								</span>
							</a>
							{#if canAct(c)}
								<span class="h-act"><IconButton icon="plus" label={actLabel(c)} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={busy === c.key || undefined} onclick={() => void quick(c)} /></span>
							{/if}
						</article>
					{/each}
				</div>
				<button class="paddle prev" class:show={!atStart} aria-label="Previous" tabindex={atStart ? -1 : 0} onclick={() => page(-1)}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M14.5 6 8.5 12l6 6" /></svg></button>
				<button class="paddle next" class:show={!atEnd} aria-label="Next" tabindex={atEnd ? -1 : 0} onclick={() => page(1)}><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9.5 6l6 6-6 6" /></svg></button>
			</div>
		{/if}

		{#if tracks}
			<section class="tracks-row" aria-labelledby="np-tracks-h">
				<header><h2 id="np-tracks-h">{tracks.title}</h2><p>{tracks.reason}</p></header>
				<ol class="tracks" data-testid="new-music-tracks">
					{#each tracks.items as raw (raw.key)}
						{@const c = live(raw)}
						<li class="track">
							<img src={musicImage(c)} alt="" loading="lazy" onerror={(e) => ((e.currentTarget as HTMLImageElement).style.visibility = 'hidden')} />
							<a class="t-words" href={musicCardHref(c)}><span class="t-name">{c.name}</span><span class="t-artist">{rowNote(tracks, c)}</span></a>
							{#if canAct(c)}<IconButton icon="plus" label={manage ? `Add ${c.artist}` : `Ask for ${c.name}`} size="sm" aria-haspopup="dialog" aria-busy={busy === c.key || undefined} onclick={() => void quick(c)} />{/if}
						</li>
					{/each}
				</ol>
			</section>
		{/if}

		{#each rows as row (row.id)}
			<div class="row" data-row={row.type}>
				<Shelf title={row.title} subtitle={row.reason} item="var(--explore-item)" items={row.items} key={(c) => c.key} placeholder="square" lines={2}>
					{#snippet card(raw: NewMusicCard)}
						{@const c = live(raw)}
						<PosterCard title={c.name} href={musicCardHref(c)} image={musicImage(c)} kind="artist" subtitle={rowNote(row, c) || undefined} shape="square" music actionOnHover
							status={c.title_id && row.type === 'np_music_library' ? 'in_library' : undefined}>
							{#snippet action()}
								{#if c.upcoming}
									<IconButton icon={c.following ? 'bellOn' : 'bell'} label={c.following ? `Following ${c.name}` : `Tell me when ${c.name} is out`} variant="glass" size="lg" aria-pressed={!!c.following} onclick={() => void follow(c)} />
								{:else if canAct(c)}
									<IconButton icon="plus" label={actLabel(c)} variant="glass" size="lg" aria-haspopup="dialog" aria-busy={busy === c.key || undefined} onclick={() => void quick(c)} />
								{/if}
							{/snippet}
						</PosterCard>
					{/snippet}
				</Shelf>
			</div>
		{/each}

		<nav class="more" aria-labelledby="np-more-h">
			<h2 id="np-more-h">Explore more</h2>
			{#each [['genre', 'Genres'], ['decade', 'Decades'], ['mood', 'Moods']] as [g, label] (g)}
				{@const list = links.filter((l) => l.group === g)}
				{#if list.length}
					<div class="more-group" role="group" aria-label={label}>
						{#each list as l (l.label)}<span class="cap"><Chip label={l.label} onclick={() => void goto(l.href)} /></span>{/each}
					</div>
				{/if}
			{/each}
		</nav>
	</section>
{/if}

{#if manage}
	<AddArtistDialog bind:this={addArtist} {folders} {rules} {defaults} onadded={added} />
{:else if mayAsk}
	<AskArtistDialog bind:this={askArtist} onasked={asked} onrefused={(a, body) => typeof body.title_id === 'number' && (addedHere = new Map([...addedHere, [a.mbid, body.title_id]]))} />
{/if}

<style>
	.music-new { display: flex; flex-direction: column; gap: 32px; }
	/* Hero: a shelf of editorial cards, 3 to a screen with a peek of the next. */
	.hero-wrap { position: relative; --h-cols: calc(31.5% - 13.33px); }
	.hero { display: grid; grid-auto-flow: column; grid-auto-columns: var(--h-cols); gap: 20px; overflow-x: auto; scroll-snap-type: x mandatory; scroll-behavior: smooth; scrollbar-width: none; padding: 4px 2px 10px; }
	.hero::-webkit-scrollbar { display: none; }
	.h-card { position: relative; scroll-snap-align: start; min-width: 0; }
	.h-link { display: flex; flex-direction: column; color: var(--text); text-decoration: none; border-radius: 12px; }
	.h-link:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.eyebrow { font: 600 11px/1.3 var(--font-ui); letter-spacing: 0.06em; text-transform: uppercase; color: var(--text-muted); }
	.h-title { margin-top: 2px; font-size: 17px; line-height: 1.3; font-weight: 400; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.h-artist { font-size: 17px; line-height: 1.3; color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; margin-bottom: 8px; }
	.art { position: relative; display: block; aspect-ratio: 16 / 9; border-radius: 10px; overflow: hidden; background: var(--surface-2); transition: transform 0.25s ease, filter 0.25s ease, box-shadow 0.25s ease; }
	.wide { width: 100%; height: 100%; object-fit: cover; }
	.shade { position: absolute; inset: 0; background: linear-gradient(to top, rgb(0 0 0 / 0.6), transparent 45%); }
	.note { position: absolute; left: 12px; right: 84px; bottom: 10px; color: #fff; font-size: 13px; line-height: 1.3; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.cover { position: absolute; right: 12px; bottom: 10px; width: 60px; height: 60px; border-radius: 4px; object-fit: cover; box-shadow: 0 2px 10px rgb(0 0 0 / 0.4); }
	.h-card:hover .art, .h-link:focus-visible .art { transform: translateY(-3px); filter: brightness(1.08); box-shadow: 0 10px 24px rgb(0 0 0 / 0.22); }
	.h-act { position: absolute; top: calc(4.6em + 8px); right: 10px; opacity: 0; transition: opacity 0.2s ease; }
	.h-card:hover .h-act, .h-card:focus-within .h-act { opacity: 1; }
	.paddle { position: absolute; top: calc(50% + 2.3em); transform: translateY(-50%); width: 36px; height: 36px; border-radius: 50%; border: 0; display: grid; place-items: center; cursor: pointer; background: color-mix(in srgb, var(--surface-1, var(--bg)) 86%, transparent); color: var(--text); box-shadow: 0 2px 10px rgb(0 0 0 / 0.25); opacity: 0; pointer-events: none; transition: opacity 0.25s ease; z-index: 2; }
	.paddle svg { width: 18px; height: 18px; fill: none; stroke: currentColor; stroke-width: 2; stroke-linecap: round; stroke-linejoin: round; }
	.paddle.prev { left: -10px; }
	.paddle.next { right: -10px; }
	.hero-wrap:hover .paddle.show, .paddle.show:focus-visible { opacity: 1; pointer-events: auto; }
	@media (hover: none) { .h-act { opacity: 1; } }
	@media (max-width: 1023px) { .hero-wrap { --h-cols: calc(48% - 10px); } }
	@media (max-width: 640px) { .hero-wrap { --h-cols: calc(100% / 1.1 - 10px); } .paddle { display: none; } }
	@media (prefers-reduced-motion: reduce) { .hero { scroll-behavior: auto; } .art, .paddle, .h-act { transition: none; } .h-card:hover .art { transform: none; } }
	/* Fresh tracks: a compact list, four lines to a column. */
	.tracks-row header h2, .more h2 { margin: 0; font: var(--t-title-3, 600 20px/1.3 var(--font-ui)); }
	.tracks-row header p { margin: 2px 0 10px; color: var(--text-muted); font-size: 14px; }
	.tracks { list-style: none; margin: 0; padding: 0 0 6px; display: grid; grid-auto-flow: column; grid-template-rows: repeat(4, auto); grid-auto-columns: calc(25% - 12px); column-gap: 16px; overflow-x: auto; scroll-snap-type: x mandatory; scrollbar-width: none; }
	.tracks::-webkit-scrollbar { display: none; }
	.track { display: flex; align-items: center; gap: 10px; min-width: 0; padding: 6px 0; border-bottom: 1px solid var(--border, color-mix(in srgb, var(--text) 10%, transparent)); scroll-snap-align: start; }
	.track img { flex: none; width: 44px; height: 44px; border-radius: 4px; object-fit: cover; background: var(--surface-2); }
	.t-words { flex: 1; min-width: 0; display: flex; flex-direction: column; color: var(--text); text-decoration: none; }
	.t-name, .t-artist { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; font-size: 14px; line-height: 1.35; }
	.t-artist { color: var(--text-muted); font-size: 13px; }
	@media (max-width: 1023px) { .tracks { grid-auto-columns: calc(50% - 8px); } }
	@media (max-width: 640px) { .tracks { grid-auto-columns: 88%; } }
	.row { position: relative; min-width: 0; }
	.more { display: flex; flex-direction: column; gap: 10px; }
	.more-group { display: flex; flex-wrap: wrap; gap: 8px; }
	.cap { display: contents; text-transform: capitalize; }
	.loading { display: flex; flex-direction: column; gap: 12px; }
	.hero-sk { display: grid; grid-template-columns: repeat(3, 1fr); gap: 20px; }
	.hero-sk span { aspect-ratio: 16 / 9; border-radius: 10px; background: var(--surface-2); margin-top: 60px; }
	.hint { color: var(--text-muted); font-size: 14px; margin: 0; }
	@media (max-width: 640px) { .hero-sk { grid-template-columns: 1fr; } .hero-sk span:nth-child(n + 2) { display: none; } }
</style>
