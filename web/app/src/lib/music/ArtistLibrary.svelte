<script lang="ts">
	// This lives on the artist page's dark panel in both themes, so status text uses the *-on-art colours.
	import { SvelteSet } from 'svelte/reactivity';
	import Icon from '#lib/Icon.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import ReleaseSheet from '#lib/ReleaseSheet.svelte';
	import DeleteSheet from '#lib/DeleteSheet.svelte';
	import EditTitleSheet from '#lib/EditTitleSheet.svelte';
	import RenamePreview from '#lib/RenamePreview.svelte';
	import { api, message } from '#lib/api.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { ago, bytes } from '#lib/format.ts';
	import type { AlbumSummary, TitleView, TrackView } from '#lib/types.ts';
	import Button from '#lib/ui/Button.svelte';
	import { to } from '#lib/paths.ts';
	import { albumChoices, albumCover, albumStateOf, extraKinds, mainKinds, newAlbumChoices, pressingStatuses, toggle as toggleKind, word } from '#lib/music/music.ts';

	// An artist's Library tab: what you have, the albums grouped by kind (closed
	// until opened, with their tracks and a link to the album page), keep up to
	// date, search and pick per album and track, activity, settings (album kinds
	// too) and delete. Same pattern as a show's seasons. `stamp` changes when the
	// page hears the artist changed, so the tab reads again.
	let { titleId, manage = false, stamp = 0, ondeleted }: { titleId: number; manage?: boolean; stamp?: number; ondeleted?: () => void } = $props();

	let title = $state<TitleView | null>(null);
	let albums = $state<AlbumSummary[]>([]);
	let events = $state<{ ID: number; At?: string; Kind?: string; Summary?: string }[]>([]);
	let busy = $state('');
	let show = $state<'included' | 'left_out' | 'all'>('included');
	let allEvents = $state(false);
	const open = new SvelteSet<number>();
	let tracks = $state<Record<number, TrackView[]>>({});

	async function load() {
		try {
			const [t, a, e] = await Promise.all([
				api.get<TitleView>(`/titles/${titleId}`),
				api.get<{ albums: AlbumSummary[] }>(`/titles/${titleId}/albums`),
				api.get<{ events?: typeof events }>(`/events?title=${titleId}&limit=20`).catch(() => ({ events: [] }))
			]);
			title = t;
			albums = a.albums ?? [];
			events = e.events ?? [];
		} catch (err) {
			toasts.push(message(err), 'fail');
		}
	}
	$effect(() => {
		void titleId;
		void stamp;
		void load();
	});

	async function loadTracks(id: number) {
		try {
			const r = await api.get<{ tracks: TrackView[] }>(`/albums/${id}/tracks`);
			tracks = { ...tracks, [id]: r.tracks ?? [] };
		} catch {
			tracks = { ...tracks, [id]: [] };
		}
	}
	function toggle(id: number) {
		if (open.has(id)) return void open.delete(id);
		open.add(id);
		if (!tracks[id]) void loadTracks(id);
	}

	// Lidarr-style groups, in this order; anything else falls under Other.
	const KINDS: [string, string][] = [['album', 'Albums'], ['ep', 'EPs'], ['single', 'Singles'], ['live', 'Live'], ['compilation', 'Compilations'], ['other', 'Other']];
	const kindOf = (a: AlbumSummary) => (a.secondary_types?.includes('live') ? 'live' : a.secondary_types?.includes('compilation') ? 'compilation' : ['album', 'ep', 'single'].includes(a.type) ? a.type : 'other');
	const kept = $derived(albums.filter((a) => a.included));
	const leftOut = $derived(albums.length - kept.length);
	const shown = $derived(show === 'all' ? albums : show === 'included' ? kept : albums.filter((a) => !a.included));
	const groups = $derived(
		KINDS.map(([k, label]) => ({ k, label, list: shown.filter((a) => kindOf(a) === k).sort((x, y) => (y.release_date ?? '').localeCompare(x.release_date ?? '')) })).filter((g) => g.list.length)
	);
	const have = $derived(kept.reduce((n, a) => n + a.in_library, 0));
	const total = $derived(kept.reduce((n, a) => n + a.tracks, 0));
	const missing = $derived(kept.reduce((n, a) => n + a.missing, 0));
	const fullAlbums = $derived(kept.filter((a) => a.tracks > 0 && a.in_library >= a.tracks).length);
	const facts = $derived.by(() => {
		const t = title;
		if (!t) return [];
		const out: { k: string; icon: 'folder' | 'archive' | 'star' | 'check' | 'x' | 'music'; v: string; title?: string }[] = [];
		const folder = String(t.folder_path ?? '').split(/[\\/]/).filter(Boolean).pop();
		if (folder) out.push({ k: 'folder', icon: 'folder', v: folder, title: t.folder_path ?? '' });
		if (t.size_bytes) out.push({ k: 'size', icon: 'archive', v: bytes(t.size_bytes) });
		if (t.pick_rules_name) out.push({ k: 'rules', icon: 'star', v: t.pick_rules_name });
		out.push({ k: 'keep', icon: t.keep_up_to_date ? 'check' : 'x', v: t.keep_up_to_date ? 'Kept up to date' : 'Not kept up to date' });
		out.push({ k: 'albums', icon: 'music', v: `${kept.length} of ${albums.length} releases kept` });
		return out;
	});

	const year = (iso?: string) => (iso ? iso.slice(0, 4) : '');
	const len = (ms?: number) => {
		if (!ms) return '';
		const sec = Math.round(ms / 1000);
		return `${Math.floor(sec / 60)}:${String(sec % 60).padStart(2, '0')}`;
	};
	const tone = (st = '') => (['have', 'imported', 'in_library', 'kept'].includes(st) ? 'have' : ['downloading', 'importing', 'searching'].includes(st) ? 'downloading' : st === 'failed' ? 'failed' : 'missing');

	async function run(key: string, fn: () => Promise<unknown>, done?: string) {
		busy = key;
		try {
			await fn();
			if (done) toasts.push(done, 'ok');
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = '';
		}
	}
	const search = (body: Record<string, unknown>, key: string, done: string) => run(key, () => api.post(`/titles/${titleId}/search`, body), done);
	const keepAlbum = (a: AlbumSummary) =>
		run(
			`k${a.id}`,
			async () => {
				await api.put(`/albums/${a.id}`, { keep_up_to_date: !a.keep_up_to_date });
				await load();
			},
			`${a.name}: ${a.keep_up_to_date ? 'HolyArrs stops looking after it.' : 'HolyArrs keeps it up to date.'}`
		);
	const keepTitle = () =>
		run(
			'keep',
			async () => {
				if (!title) return;
				await api.put(`/titles/${titleId}`, { keep_up_to_date: !title.keep_up_to_date });
				await load();
			},
			title?.keep_up_to_date ? 'HolyArrs stops looking after this artist.' : 'HolyArrs keeps this artist up to date.'
		);
	const refresh = () => run('refresh', () => api.post(`/titles/${titleId}/refresh`), 'Refreshing albums, tracks, names and pictures.');
	const albumPage = (a: AlbumSummary) => to(`/library/${titleId}/albums/${a.id}`);

	// Album kinds: what counts for this artist, and what happens to albums
	// MusicBrainz lists later. Saving brings the newly included albums in.
	let kindsOpen = $state(false);
	let eNew = $state('all');
	let eTypes = $state<string[]>([]);
	let eExtras = $state<string[]>([]);
	let eStatuses = $state<string[]>([]);
	let kindsError = $state('');
	let saving = $state(false);
	const kindsLine = $derived([word(newAlbumChoices, title?.artist?.new_albums), (title?.artist?.album_types ?? []).map((k) => word(mainKinds, k)).join(', ')].filter(Boolean).join(' · '));
	function openKinds() {
		const ar = title?.artist;
		kindsError = '';
		eNew = ar?.new_albums ?? 'all';
		eTypes = [...(ar?.album_types ?? [])];
		eExtras = [...(ar?.secondary_types ?? [])];
		eStatuses = [...(ar?.release_statuses ?? [])];
		kindsOpen = true;
	}
	async function saveKinds(e: SubmitEvent) {
		e.preventDefault();
		saving = true;
		kindsError = '';
		try {
			await api.put(`/titles/${titleId}/artist`, { new_albums: eNew, album_types: eTypes, secondary_types: eExtras, release_statuses: eStatuses });
			kindsOpen = false;
			toasts.push('Saved. A refresh brings in the albums the new kinds include.', 'ok');
			await load();
		} catch (err) {
			kindsError = message(err);
		} finally {
			saving = false;
		}
	}

	let pickOpen = $state(false);
	let pickScope = $state<{ label: string; query: string; body: Record<string, unknown> } | null>(null);
	const pick = (s: typeof pickScope) => ((pickScope = s), (pickOpen = true));
	let editOpen = $state(false);
	let deleteOpen = $state(false);
	let renameOpen = $state(false);
</script>

{#if title}
	{@const t = title}
	<div class="al" data-testid="artist-library">
		<div class="al-main">
			<section class="al-status">
				<div class="al-st-text">
					<p class="al-st-title"><span class="dot {missing ? 'part' : 'have'}" aria-hidden="true"></span>{have} of {total} tracks</p>
					<p class="al-st-sub">{fullAlbums} full {fullAlbums === 1 ? 'release' : 'releases'}{missing && t.keep_up_to_date ? ' · the rest are grabbed when found' : ''}</p>
					<p class="al-facts">{#each facts as f (f.k)}<span class="al-fact" title={f.title ?? f.v}><Icon name={f.icon} size={13} />{f.v}</span>{/each}</p>
				</div>
				{#if manage}
					{#if missing}<button type="button" class="al-btn primary" disabled={busy === 'all'} onclick={() => search({}, 'all', 'Looking for every missing track.')}><Icon name="search" size={16} />Get all missing</button>{/if}
					<button type="button" class="al-btn" onclick={() => pick(null)}><Icon name="grid" size={16} />Pick a release</button>
				{/if}
			</section>

			<div class="al-head">
				<h3 class="al-h">Releases</h3>
				<span class="al-head-acts">
					{#if leftOut}
						<span class="al-head-acts" role="group" aria-label="Releases shown">
							<button type="button" class="al-btn sm" class:on={show === 'included'} aria-pressed={show === 'included'} onclick={() => (show = 'included')}>Included</button>
							<button type="button" class="al-btn sm" class:on={show === 'left_out'} aria-pressed={show === 'left_out'} onclick={() => (show = 'left_out')}>Not included {leftOut}</button>
							<button type="button" class="al-btn sm" class:on={show === 'all'} aria-pressed={show === 'all'} onclick={() => (show = 'all')}>All {albums.length}</button>
						</span>
					{/if}
					<button type="button" class="al-btn sm" onclick={() => (open.size ? open.clear() : shown.forEach((a) => !open.has(a.id) && toggle(a.id)))}>{open.size ? 'Collapse all' : 'Expand all'}</button>
				</span>
			</div>

			{#each groups as g (g.k)}
				<p class="al-group">{g.label} <span>{g.list.length}</span></p>
				<div class="al-list">
					{#each g.list as a (a.id)}
						{@const isOpen = open.has(a.id)}
						{@const full = a.tracks > 0 && a.in_library >= a.tracks}
						<section class="al-row" class:open={isOpen} class:off={!a.included} data-testid="album">
							<div class="al-row-head">
								{#if manage}<button type="button" class="al-keep" class:on={a.keep_up_to_date} aria-pressed={a.keep_up_to_date} aria-label="Keep {a.name} up to date" title={!a.included ? 'Not included by the album kinds' : a.keep_up_to_date ? 'Kept up to date. Click to stop' : 'Not kept up to date. Click to keep'} disabled={busy === `k${a.id}` || !a.included} onclick={() => keepAlbum(a)}><Icon name="check" size={13} /></button>{/if}
								<button type="button" class="al-row-main" aria-expanded={isOpen} onclick={() => toggle(a.id)}>
									<img class="al-cover" src={albumCover(a.id)} alt="" loading="lazy" decoding="async" onerror={(e) => ((e.currentTarget as HTMLImageElement).style.visibility = 'hidden')} />
									<span class="al-name"><b>{a.name}</b><small>{[year(a.release_date), a.tracks ? `${a.tracks} tracks` : '', a.not_out_yet && albumStateOf(a).label !== 'Not out yet' ? 'Not out yet' : '', a.dropped ? 'no longer listed' : ''].filter(Boolean).join(' · ')} · <span data-testid="album-state">{albumStateOf(a).label}</span></small></span>
									{#if a.included}<span class="al-count" class:full class:part={!full && a.in_library > 0} class:none={!a.in_library && a.missing > 0} data-testid="album-count">{a.in_library} / {a.tracks}</span>{/if}
									<span class="al-chev" aria-hidden="true"><Icon name="chevronDown" size={16} /></span>
								</button>
								{#if manage}
									<span class="al-acts">
										<button type="button" class="al-ico" title="Get the missing tracks" aria-label="Get the missing tracks of {a.name}" disabled={!a.missing || busy === `a${a.id}`} onclick={() => search({ album_id: a.id }, `a${a.id}`, `Looking for ${a.name}.`)}><Icon name="search" size={15} /></button>
										<button type="button" class="al-ico" title="Pick a release for this album" aria-label="Pick a release for {a.name}" onclick={() => pick({ label: a.name, query: `album_id=${a.id}`, body: { album_id: a.id } })}><Icon name="grid" size={15} /></button>
									</span>
								{/if}
							</div>
							{#if isOpen}
								<ol class="al-tracks">
									{#each tracks[a.id] ?? [] as tr (tr.id)}
										{@const st = tr.slots[0]}
										{@const tn = tone(st?.state)}
										<li class="al-tr {tn}">
											<span class="al-tr-n">{tr.disc > 1 ? `${tr.disc}-` : ''}{tr.number}</span>
											<span class="al-tr-name">{tr.name ?? `Track ${tr.number}`}</span>
											<span class="al-tr-len">{len(tr.length_ms)}</span>
											<span class="al-tr-st {tn}"><span class="dot {tn}" aria-hidden="true"></span>{st?.state_label ?? 'Missing'}</span>
											{#if manage && st}<button type="button" class="al-ico" title="Look for it now" aria-label="Look for {tr.name}" disabled={tn === 'have' || tn === 'downloading' || busy === `t${tr.id}`} onclick={() => search({ slot_id: st.id }, `t${tr.id}`, `Looking for ${tr.name ?? 'the track'}.`)}><Icon name="search" size={14} /></button>{/if}
										</li>
									{:else}
										{#each [0, 1, 2] as i (i)}<li class="al-tr skel"></li>{/each}
									{/each}
								</ol>
								<a class="al-open" href={albumPage(a)} aria-label="Open the album page of {a.name}">Open the album page<Icon name="chevronRight" size={15} /></a>
							{/if}
						</section>
					{/each}
				</div>
			{:else}
				<p class="muted">{show === 'left_out' ? 'Nothing here.' : 'No releases included yet. Album kinds in Settings picks which count.'}</p>
			{/each}
		</div>

		<aside class="al-side">
			<h3 class="al-h">History</h3>
			<ol class="al-tl">
				{#each allEvents ? events : events.slice(0, 6) as e (e.ID)}<li><span>{(e.Summary ?? e.Kind ?? '').replace(/\s+(from|on|via) (MusicBrainz|TMDB|TVmaze|TheTVDB|Wikipedia|Wikidata|fanart\.tv)\b/gi, '').replace(/^./, (c) => c.toUpperCase())}</span><span class="muted">{e.At ? ago(e.At) : ''}</span></li>
				{:else}<li class="muted">Nothing has happened yet.</li>{/each}
			</ol>
			<p class="al-links">
				{#if events.length > 6}<button type="button" class="al-link" onclick={() => (allEvents = !allEvents)}>{allEvents ? 'Show less' : `Show all ${events.length}`}</button>{/if}
				<a class="al-link" href={to(`/downloads/history?title=${titleId}`)}>All history</a>
			</p>

			{#if manage}
				<h3 class="al-h">Settings</h3>
				<div class="al-rows">
					<button type="button" class="al-rowbtn" disabled={busy === 'keep'} onclick={keepTitle}><span><b>Keep up to date</b><small>Grab new releases and upgrade when a better copy shows up</small></span><span class="al-switch" class:on={t.keep_up_to_date} aria-hidden="true"></span></button>
					<button type="button" class="al-rowbtn" onclick={() => (editOpen = true)}><span><b>Edit</b><small>{t.pick_rules_name ?? 'Quality'} · folder</small></span><Icon name="tools" size={16} /></button>
					<button type="button" class="al-rowbtn" onclick={openKinds}><span><b>Album kinds</b><small>{kindsLine}</small></span><Icon name="music" size={16} /></button>
					<button type="button" class="al-rowbtn" onclick={() => (renameOpen = true)}><span><b>Rename files</b><small>See the new names before anything changes</small></span><Icon name="chevronRight" size={16} /></button>
					<button type="button" class="al-rowbtn" disabled={busy === 'refresh'} onclick={refresh}><span><b>Refresh details</b><small>Albums, tracks, names and pictures{t.refreshed_at ? ` · ${ago(t.refreshed_at)}` : ''}</small></span><Icon name="refresh" size={16} /></button>
				</div>
				<div class="al-rows danger">
					<button type="button" class="al-rowbtn" onclick={() => (deleteOpen = true)}><span><b>Delete artist</b><small>Choose whether the files go too</small></span><Icon name="trash" size={16} /></button>
				</div>
			{/if}
		</aside>
	</div>

	{#if manage}
		<ReleaseSheet bind:open={pickOpen} {titleId} name={t.name} scope={pickScope} onchosen={load} />
		<EditTitleSheet bind:open={editOpen} title={t} onsaved={load} />
		<DeleteSheet bind:open={deleteOpen} {titleId} name={t.name} kind="artist" ondone={() => ondeleted?.()} />
		<Sheet bind:open={renameOpen} title="Rename files" description={t.name} placement="right" size="md">
			{#if renameOpen}<RenamePreview {titleId} onDone={() => { renameOpen = false; void load(); }} />{/if}
		</Sheet>
		<Sheet bind:open={kindsOpen} title="Album kinds" description={t.name} placement="right" size="md">
			<form class="al-kinds" onsubmit={saveKinds}>
				<div class="field">
					<label for="al-new">New albums</label>
					<select id="al-new" class="input" bind:value={eNew}>
						{#each newAlbumChoices as [v, l] (v)}<option value={v}>{l}</option>{/each}
					</select>
				</div>
				<fieldset class="al-kind-set">
					<legend><b>Album kinds</b></legend>
					<div class="al-boxes">
						{#each mainKinds as [v, l] (v)}<label class="al-box"><input type="checkbox" checked={eTypes.includes(v)} onchange={(e) => (eTypes = toggleKind(eTypes, v, e.currentTarget.checked, mainKinds))} /> {l}</label>{/each}
					</div>
				</fieldset>
				<fieldset class="al-kind-set">
					<legend><b>Extra kinds</b></legend>
					<div class="al-boxes">
						{#each extraKinds as [v, l] (v)}<label class="al-box"><input type="checkbox" checked={eExtras.includes(v)} onchange={(e) => (eExtras = toggleKind(eExtras, v, e.currentTarget.checked, extraKinds))} /> {l}</label>{/each}
					</div>
				</fieldset>
				<fieldset class="al-kind-set">
					<legend><b>Pressings that count</b></legend>
					<div class="al-boxes">
						{#each pressingStatuses as [v, l] (v)}<label class="al-box"><input type="checkbox" checked={eStatuses.includes(v)} onchange={(e) => (eStatuses = toggleKind(eStatuses, v, e.currentTarget.checked, pressingStatuses))} /> {l}</label>{/each}
					</div>
				</fieldset>
				{#if t.artist?.album_choice}<p class="hint">Picked when added: {word(albumChoices, t.artist.album_choice)}</p>{/if}
				<p class="hint">Albums the new kinds leave out stay listed under Not included; their files stay where they are.</p>
				{#if kindsError}<p class="form-error" role="alert">{kindsError}</p>{/if}
				<div class="al-kinds-acts">
					<Button variant="plain" onclick={() => (kindsOpen = false)}>Cancel</Button>
					<Button variant="primary" type="submit" disabled={saving || !eTypes.length || !eExtras.length || !eStatuses.length} busy={saving}>Save</Button>
				</div>
			</form>
		</Sheet>
	{/if}
{:else}
	<div class="al-skel" aria-busy="true"></div>
{/if}

<style>
	.al { display: grid; gap: 0 40px; grid-template-columns: minmax(0, 1fr); margin-top: 8px; }
	.al-main, .al-side { display: grid; align-content: start; min-width: 0; }
	@media (min-width: 1100px) {
		.al { grid-template-columns: minmax(0, 1fr) minmax(300px, 360px); }
		.al-side { position: sticky; top: calc(var(--topbar, 56px) + 16px); }
		.al-side > .al-h:first-child { margin-top: 0; }
	}
	.al-skel { height: 220px; border-radius: 18px; background: var(--surface-2); }
	.al-h { margin: 28px 0 12px; font: 700 20px/1.2 var(--font-ui); letter-spacing: -.01em; }
	.al-status { display: flex; flex-wrap: wrap; align-items: center; gap: 12px 12px; padding: 20px 22px; border-radius: 18px; background: var(--surface-2); box-shadow: inset 0 0 0 .5px color-mix(in srgb, var(--text) 10%, transparent); }
	.al-st-text { flex: 1 1 320px; min-width: 0; }
	.al-st-title { display: flex; align-items: center; gap: 10px; margin: 0; font: 700 22px/1.2 var(--font-ui); letter-spacing: -.01em; }
	.al-st-sub { margin: 6px 0 0 20px; color: var(--text-muted); font: 500 14px/1.4 var(--font-ui); }
	.al-facts { display: flex; flex-wrap: wrap; gap: 6px; margin: 12px 0 0 20px; }
	.al-fact { display: inline-flex; align-items: center; gap: 6px; max-width: 260px; height: 26px; padding: 0 10px; border-radius: 8px; background: var(--surface-1); color: var(--text-muted); font: 600 12.5px/1 var(--font-ui); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.dot { flex: none; width: 9px; height: 9px; border-radius: 99px; background: var(--text-muted); }
	.dot.have { background: var(--ok); }
	.dot.part { background: var(--wait); }
	.dot.downloading { background: var(--info); }
	.dot.failed { background: var(--fail); }
	.al-btn { display: inline-flex; align-items: center; gap: 8px; height: 40px; padding: 0 18px; border: 0; border-radius: 999px; background: color-mix(in srgb, var(--text) 8%, transparent); color: var(--text); font: 600 14px/1 var(--font-ui); white-space: nowrap; cursor: pointer; }
	.al-btn.primary { background: var(--text); color: var(--bg); }
	.al-btn.sm { height: 34px; padding: 0 14px; font-size: 13px; }
	.al-btn.on { background: var(--text); color: var(--bg); }
	.al-head { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 0 12px; }
	.al-head .al-btn.sm { margin-top: 16px; }
	.al-head-acts { display: flex; flex-wrap: wrap; gap: 0 8px; }
	.al-open { display: flex; align-items: center; justify-content: flex-end; gap: 4px; min-height: var(--tap); padding: 0 16px; border-top: .5px solid color-mix(in srgb, var(--text) 8%, transparent); color: var(--accent-text); font: 600 14px/1 var(--font-ui); text-decoration: none; }
	.al-links { display: flex; flex-wrap: wrap; gap: 16px; margin: 0; }
	.al-links .al-link { text-decoration: none; }
	.al-kinds { display: grid; gap: var(--s-4); }
	.al-kind-set { margin: 0; padding: 0; border: 0; }
	.al-kind-set legend { margin-bottom: var(--s-2); font-size: var(--text-sm); }
	.al-boxes { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: var(--s-2); }
	.al-box { display: flex; align-items: center; gap: var(--s-2); min-height: var(--tap); font-size: var(--text-sm); }
	.al-kinds-acts { display: flex; justify-content: flex-end; gap: var(--s-2); }
	.al-group { margin: 14px 0 8px; color: var(--text-muted); font: 700 12px/1 var(--font-ui); letter-spacing: .06em; text-transform: uppercase; }
	.al-group span { margin-left: 4px; opacity: .7; }
	.al-list { display: grid; gap: 8px; }
	.al-row { border-radius: 14px; background: var(--surface-2); overflow: hidden; }
	.al-row.off { opacity: .6; }
	.al-row-head { display: flex; align-items: center; gap: 8px; padding: 0 10px 0 12px; min-height: 64px; }
	.al-row-main { flex: 1; min-width: 0; display: flex; align-items: center; gap: 12px; height: 64px; padding: 0 4px; border: 0; background: none; color: var(--text); font: inherit; text-align: left; cursor: pointer; }
	.al-cover { flex: none; width: 46px; height: 46px; border-radius: 6px; object-fit: cover; background: color-mix(in srgb, var(--text) 8%, transparent); }
	.al-name { display: grid; gap: 3px; min-width: 0; }
	.al-name b { font: 650 15px/1.2 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.al-name small { color: var(--text-muted); font: 500 12.5px/1.2 var(--font-ui); }
	.al-count { flex: none; display: inline-flex; align-items: center; height: 22px; padding: 0 8px; border-radius: 99px; background: color-mix(in srgb, var(--text) 9%, transparent); color: var(--text); font: 700 12px/1 var(--font-ui); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.al-count.full { background: color-mix(in srgb, var(--ok) 16%, transparent); color: var(--ok-on-art); }
	.al-count.part { background: color-mix(in srgb, var(--wait) 16%, transparent); color: var(--wait-on-art); }
	.al-count.none { background: color-mix(in srgb, var(--fail) 14%, transparent); color: var(--text); }
	.al-chev { margin-left: auto; display: grid; color: var(--text-muted); transition: transform var(--dur-fast) var(--ease-out); }
	.al-row.open .al-chev { transform: rotate(180deg); }
	.al-acts { display: flex; gap: 2px; }
	.al-ico { display: grid; place-items: center; width: 32px; height: 32px; border: 0; border-radius: 99px; background: none; color: var(--text-muted); cursor: pointer; }
	.al-ico:hover:not(:disabled) { background: color-mix(in srgb, var(--text) 9%, transparent); color: var(--text); }
	.al-ico:disabled { opacity: .3; cursor: default; }
	/* The box is small; its hit area reaches 44px (the tap size). */
	.al-keep::after { content: ''; position: absolute; inset: -11px; }
	.al-keep:disabled { opacity: .35; cursor: default; }
	.al-keep { position: relative; flex: none; display: grid; place-items: center; width: 22px; height: 22px; padding: 0; border: 1.5px solid color-mix(in srgb, var(--text) 30%, transparent); border-radius: 6px; background: none; color: transparent; cursor: pointer; }
	.al-keep.on { border-color: var(--ok); background: var(--ok); color: #fff; }
	.al-tracks { display: grid; margin: 0; padding: 0 8px 8px; list-style: none; border-top: .5px solid color-mix(in srgb, var(--text) 8%, transparent); }
	.al-tr { display: grid; grid-template-columns: 34px minmax(0, 1fr) auto auto 32px; align-items: center; gap: 12px; min-height: 42px; padding: 0 8px; border-radius: 8px; font: 500 14px/1.2 var(--font-ui); }
	.al-tr:hover { background: color-mix(in srgb, var(--text) 5%, transparent); }
	.al-tr.skel { height: 42px; background: color-mix(in srgb, var(--text) 5%, transparent); margin-top: 4px; }
	.al-tr-n { color: var(--text-muted); font-variant-numeric: tabular-nums; text-align: right; }
	.al-tr-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.al-tr-len { color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.al-tr-st { display: inline-flex; align-items: center; gap: 7px; height: 24px; padding: 0 9px; border-radius: 99px; background: color-mix(in srgb, var(--text) 7%, transparent); color: var(--text); font: 600 12px/1 var(--font-ui); white-space: nowrap; }
	.al-tr-st.have { color: var(--ok-on-art); background: color-mix(in srgb, var(--ok) 12%, transparent); }
	.al-tr-st.downloading { color: var(--info-on-art); background: color-mix(in srgb, var(--info) 12%, transparent); }
	.al-tr-st.failed { color: var(--fail-on-art); background: color-mix(in srgb, var(--fail) 12%, transparent); }
	.al-tl { display: grid; gap: 10px; margin: 0; padding: 0; list-style: none; font: 500 14px/1.35 var(--font-ui); }
	.al-tl li { display: flex; justify-content: space-between; gap: 12px; }
	.al-link { justify-self: start; margin-top: 8px; padding: 0; border: 0; background: none; color: inherit; font: 600 14px/1 var(--font-ui); text-decoration: underline; text-underline-offset: 3px; cursor: pointer; }
	.al-rows { display: grid; border-radius: 16px; background: var(--surface-2); overflow: hidden; }
	.al-rows.danger { margin-top: 16px; }
	.al-rows.danger b { color: var(--on-art); }
	.al-rows.danger :global(svg) { color: var(--fail-on-art); }
	.al-rowbtn { display: flex; align-items: center; justify-content: space-between; gap: 16px; width: 100%; padding: 14px 18px; border: 0; background: none; color: var(--text); text-align: left; cursor: pointer; font: inherit; }
	.al-rowbtn + .al-rowbtn { box-shadow: inset 0 .5px 0 color-mix(in srgb, var(--text) 10%, transparent); }
	.al-rowbtn span { display: grid; gap: 3px; min-width: 0; }
	.al-rowbtn b { font: 600 15px/1.2 var(--font-ui); }
	.al-rowbtn small { color: var(--text-muted); font: 500 13px/1.3 var(--font-ui); }
	.al-rowbtn :global(svg) { flex: none; color: var(--text-muted); }
	.al-switch { position: relative; flex: none; width: 44px; height: 26px; border-radius: 99px; background: color-mix(in srgb, var(--text) 18%, transparent); }
	.al-switch::after { content: ''; position: absolute; top: 3px; left: 3px; width: 20px; height: 20px; border-radius: 99px; background: #fff; box-shadow: 0 1px 3px rgb(0 0 0 / 25%); transition: transform var(--dur-fast) var(--ease-out); }
	.al-switch.on { background: var(--ok); }
	.al-switch.on::after { transform: translateX(18px); }
	@media (max-width: 600px) {
		.al-status { padding: 18px; }
		.al-btn:not(.sm) { flex: 1; justify-content: center; }
		.al-btn.sm { height: var(--tap); }
		.al-links .al-link { display: inline-flex; align-items: center; min-height: var(--tap); margin-top: 0; }
		.al-tr { grid-template-columns: 26px minmax(0, 1fr) auto 32px; }
		.al-tr-len { display: none; }
		.al-row-main { gap: 10px; }
		.al-cover { width: 40px; height: 40px; }
		.al-chev { display: none; }
		.al-row-head { padding: 0 6px 0 10px; gap: 6px; }
		.al-acts .al-ico { width: 28px; }
	}
</style>
