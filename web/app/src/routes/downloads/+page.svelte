<script lang="ts">
	import { to } from '#lib/paths.ts';
	import { onMount, tick } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { ago, bytes, duration, percent, tone, words } from '#lib/format.ts';
	import { episodeStateWords, manyWords, phaseTone, removeBody, removeStart, rowName, split, type DownloaderSettings, type DownloadRow, type RemoveChoice } from '#lib/downloaders.ts';
	import EmptyState from '#lib/EmptyState.svelte';
	import Poster from '#lib/Poster.svelte';
	import type { ImportRecord, TitleView } from '#lib/types.ts';

	let rows = $state<DownloadRow[]>([]);
	let imports = $state<ImportRecord[]>([]);
	let titles = $state<TitleView[]>([]);
	let loaded = $state(false);
	let error = $state('');
	let settings = $state<DownloaderSettings | null>(null);
	const parts = $derived(split(rows));
	let selectedId = $state(0);
	const selected = $derived(rows.find((r) => r.id === selectedId) ?? parts.now[0] ?? parts.past[0] ?? null);
	const titleById = $derived(new Map(titles.map((title) => [title.id, title])));
	let detailHeading: HTMLHeadingElement | undefined = $state();

	async function selectDownload(id: number) {
		selectedId = id;
		await tick();
		if (!detailHeading) return;
		detailHeading.focus({ preventScroll: true });
		if (window.matchMedia('(max-width: 980px)').matches) {
			const rect = detailHeading.getBoundingClientRect();
			if (rect.top < 72 || rect.bottom > window.innerHeight) {
				detailHeading.scrollIntoView({ behavior: 'smooth', block: 'start' });
			}
		}
	}

	async function load() {
		try {
			const [d, i] = await Promise.all([
				api.get<{ downloads: DownloadRow[] }>('/downloads/view?limit=100'),
				api.get<{ imports: ImportRecord[] }>('/imports?limit=20')
			]);
			rows = d.downloads;
			imports = i.imports;
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}

	// Live news comes often while something downloads: read again at most
	// once a second.
	let timer: ReturnType<typeof setTimeout> | undefined;
	function soon() {
		if (timer) return;
		timer = setTimeout(() => {
			timer = undefined;
			void load();
		}, 400);
	}

	onMount(() => {
		void load();
		void api.get<{ titles: TitleView[] }>('/titles').then((r) => (titles = r.titles)).catch(() => {});
		api.get<DownloaderSettings>('/settings/downloaders').then((s) => (settings = s)).catch(() => {});
		const offs = ['download', 'slot', 'import', 'resync'].map((t) => live.on(t, soon));
		return () => {
			offs.forEach((off) => off());
			clearTimeout(timer);
		};
	});

	function who(r: DownloadRow) {
		const n = rowName(r);
		return manyWords(r.kind) && r.covers ? `${n} ${r.covers}` : n;
	}

	let busy = $state(0);
	async function markFailed(r: DownloadRow) {
		if (!confirm(`Mark ${who(r)} failed? Its Release is turned down, it is removed from the Downloader and HolyArrs looks for the next Release.`)) return;
		busy = r.id;
		try {
			await api.post(`/downloads/${r.id}/mark-failed`);
			toasts.push('Marked failed. HolyArrs looks for the next Release.', 'ok');
			await load();
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = 0;
		}
	}

	// The Remove dialog.
	let removeDialog: HTMLDialogElement | undefined = $state();
	let target = $state<DownloadRow | null>(null);
	let choice = $state<RemoveChoice>(removeStart(null));
	let removeError = $state('');
	function askRemove(r: DownloadRow) {
		target = r;
		choice = removeStart(settings);
		removeError = '';
		removeDialog?.showModal();
	}
	async function remove(e: SubmitEvent) {
		e.preventDefault();
		if (!target) return;
		busy = target.id;
		try {
			await api.post(`/downloads/${target.id}/remove`, removeBody(choice));
			removeDialog?.close();
			toasts.push(choice.turn_down && choice.search_again ? 'Removed. HolyArrs looks for the next Release.' : 'Removed.', 'ok');
			await load();
		} catch (err) {
			removeError = message(err);
		} finally {
			busy = 0;
		}
	}
</script>

<svelte:head><title>Downloads · HolyArrs</title></svelte:head>

<header class="page-head">
	<div>
		<p class="eyebrow">Operations</p>
		<h1>Downloads</h1>
		<p>What your Downloaders are fetching right now, live, and what HolyArrs moved into the Library.</p>
	</div>
	<nav class="page-actions" aria-label="Download views">
		<a class="btn btn-ghost" href={to(`/downloads/decide`)}>Needs a decision</a>
		<a class="btn btn-ghost" href={to(`/downloads/history`)}>History</a>
	</nav>
</header>

{#snippet dlRow(r: DownloadRow)}
	{@const pct = percent(r.progress)}
	<li class="dl" class:selected={selected?.id === r.id} data-testid="download-row" data-phase={r.phase}>
		<span class="media-art" class:square={r.kind === 'artist'} aria-hidden="true"><Poster src={r.artwork ?? titleById.get(r.title_id ?? 0)?.artwork[0]} name={r.title || r.name} /></span>
		<div class="grow">
			<div class="top">
				<div class="names">
					{#if r.title_id}
						<a class="nm small" href={to(`/library/${r.title_id}`)}>{rowName(r)}</a>
					{:else}
						<span class="nm small">{rowName(r)}</span>
					{/if}
					{#if manyWords(r.kind) && r.covers}
						<span class="small covers" data-testid="row-covers">{r.covers}</span>
					{/if}
					<span class="xs muted ellipsis mono" title={r.name}>{r.name}</span>
				</div>
				<span class="chip {phaseTone(r.phase)}" data-testid="row-state">{r.label}</span>
			</div>
			{#if r.phase !== 'removed' && r.phase !== 'failed' && r.phase !== 'vanished'}
				<div class="bar" class:done={r.progress >= 1} role="progressbar" aria-valuenow={pct} aria-valuemin="0" aria-valuemax="100" aria-label="Progress of {who(r)}">
					<i style="transform: scaleX({pct / 100})"></i>
				</div>
			{/if}
			<div class="mono xs muted meta">
				<span data-testid="row-percent">{pct}%</span>
				{#if r.size}<span>{bytes(r.size)}</span>{/if}
				{#if r.active && r.speed}<span data-testid="row-speed">{bytes(r.speed)}/s</span>{/if}
				{#if r.active && r.eta}<span data-testid="row-eta">{duration(r.eta)} left</span>{/if}
				{#if r.version}<span>{r.version}</span>{/if}
				{#if r.downloader}<span>{r.downloader}</span>{/if}
				{#if !r.active}<span>{ago(r.updated_at)}</span>{/if}
			</div>
			{#if r.text}<p class="small txt" data-testid="row-text">{r.text}</p>{/if}
			{#if manyWords(r.kind) && r.episodes && r.episodes.length > 1}
				<details class="eps" data-testid="row-episodes">
					<summary class="small">The {r.episodes.length} {manyWords(r.kind)?.many} it covers</summary>
					<ul class="eplist">
						{#each r.episodes as e (e.slot_id)}
							{@const st = episodeStateWords(e.state)}
							<li data-testid="row-episode">
								<span class="mono xs">{e.label}</span>
								<span class="small ellipsis">{e.name ?? ''}</span>
								<span class="chip {st.tone}">{st.text}</span>
							</li>
						{/each}
					</ul>
				</details>
			{/if}
		</div>
		{#if r.can_act}
			<div class="acts">
				<button class="btn btn-quiet inspect" type="button" aria-label="Details for {who(r)}" aria-pressed={selected?.id === r.id} onclick={() => void selectDownload(r.id)}>Details</button>
				<button class="btn btn-quiet" type="button" onclick={() => markFailed(r)} disabled={busy === r.id}>Mark failed</button>
				<button class="btn btn-ghost" type="button" onclick={() => askRemove(r)} disabled={busy === r.id}>Remove</button>
			</div>
		{:else}
			<button class="btn btn-quiet inspect" type="button" aria-label="Details for {who(r)}" aria-pressed={selected?.id === r.id} onclick={() => void selectDownload(r.id)}>Details</button>
		{/if}
	</li>
{/snippet}

{#if !loaded}
	<div class="loading-grid" aria-label="Loading downloads">
		<div class="sk" style="height: 320px"></div>
		<div class="sk" style="height: 220px"></div>
	</div>
{:else if error}
	<div class="notice bad load-error" role="alert">
		<div><b>Downloads could not be loaded.</b><br /><span class="small">{error}</span></div>
		<button class="btn btn-ghost" type="button" onclick={load}>Try again</button>
	</div>
{:else if rows.length === 0 && imports.length === 0}
	<EmptyState icon="download" title="No downloads yet">
		<p>When HolyArrs chooses a Release for a movie or an episode, it sends it to a Downloader and you can watch it here, live.</p>
		{#snippet actions()}
			<a class="btn btn-dark" href={to(`/search`)}>Add media</a>
			<a class="btn btn-ghost" href={to(`/settings/downloaders`)}>Downloaders</a>
		{/snippet}
	</EmptyState>
{:else}
	<div class="download-workspace">
		<div class="download-main">
			<section class="card queue" aria-labelledby="h-d">
				<div class="card-head"><h2 id="h-d" class="h2">Now</h2><span class="count">{parts.now.length}</span></div>
				{#if parts.now.length === 0}
					<p class="small muted empty-line">Nothing is downloading.</p>
				{:else}
					<ul class="rows">
						{#each parts.now as r (r.id)}{@render dlRow(r)}{/each}
					</ul>
				{/if}
			</section>

			{#if parts.past.length}
				<section class="card queue" aria-labelledby="h-p">
					<div class="card-head"><h2 id="h-p" class="h2">Finished and stopped</h2><span class="count">{parts.past.length}</span></div>
					<ul class="rows">
						{#each parts.past as r (r.id)}{@render dlRow(r)}{/each}
					</ul>
				</section>
			{/if}

			<section class="card queue" aria-labelledby="h-i">
				<div class="card-head"><h2 id="h-i" class="h2">Imports</h2><span class="count">{imports.length}</span></div>
				{#if imports.length === 0}
					<p class="small muted empty-line">No imports yet. A finished download is moved into the Library here.</p>
				{:else}
					<ul class="rows imports">
						{#each imports as im (im.ID)}
							{@const placed = im.Result?.find((r) => r.outcome === 'placed')}
							<li>
								<div class="grow">
									<div class="ellipsis small mono">{placed?.source_file ?? im.Plan?.[0]?.source_file ?? `Import ${im.ID}`}</div>
									{#if placed?.placed_by}<div class="xs muted">Placed by {placed.placed_by}</div>{/if}
								</div>
								<span class="xs muted">{ago(im.CreatedAt)}</span>
								<span class="chip {tone(im.State)}">{words(im.State)}</span>
							</li>
						{/each}
					</ul>
				{/if}
			</section>
		</div>

		<aside class="detail card" aria-label="Selected download details">
			{#if selected}
				<div class="detail-art" class:square={selected.kind === 'artist'} aria-hidden="true"><Poster src={selected.artwork ?? titleById.get(selected.title_id ?? 0)?.artwork[0]} name={selected.title || selected.name} /></div>
				<div class="detail-head">
					<p class="eyebrow">Selected download</p>
					<span class="chip {phaseTone(selected.phase)}">{selected.label}</span>
				</div>
				<h2 class="h2 detail-title" bind:this={detailHeading} tabindex="-1">{rowName(selected)}</h2>
				{#if manyWords(selected.kind) && selected.covers}<p class="small">{selected.covers}</p>{/if}
				<p class="mono xs muted detail-path">{selected.name}</p>
				<dl class="detail-facts">
					<dt>Progress</dt><dd>{percent(selected.progress)}%</dd>
					{#if selected.size}<dt>Size</dt><dd>{bytes(selected.size)}</dd>{/if}
					{#if selected.active && selected.speed}<dt>Speed</dt><dd>{bytes(selected.speed)}/s</dd>{/if}
					{#if selected.active && selected.eta}<dt>Time left</dt><dd>{duration(selected.eta)}</dd>{/if}
					{#if selected.downloader}<dt>Downloader</dt><dd>{selected.downloader}</dd>{/if}
					{#if selected.version}<dt>Version</dt><dd>{selected.version}</dd>{/if}
				</dl>
				{#if selected.text}<p class="small detail-note">{selected.text}</p>{/if}
				{#if selected.can_act}
					<div class="detail-actions">
						<button class="btn btn-ghost" type="button" onclick={() => markFailed(selected)} disabled={busy === selected.id}>Mark failed</button>
						<button class="btn btn-quiet" type="button" onclick={() => askRemove(selected)} disabled={busy === selected.id}>Remove</button>
					</div>
				{/if}
			{:else}
				<p class="small muted">Choose a download to inspect its state and available actions.</p>
			{/if}
		</aside>
	</div>
{/if}

<dialog bind:this={removeDialog} aria-labelledby="remove-title" class="modal">
	{#if target}
		<form onsubmit={remove}>
			<h2 id="remove-title" class="h2">Remove this download?</h2>
			<p class="small" style="margin-top: var(--s-3)">{who(target)}</p>
			<p class="mono xs muted path">{target.name}</p>
			<label class="checkline">
				<input type="checkbox" bind:checked={choice.delete_files} />
				<span><b>Delete its files</b><br /><span class="hint">Off: {target.downloader || 'the Downloader'} keeps what it fetched.</span></span>
			</label>
			<label class="checkline">
				<input type="checkbox" bind:checked={choice.turn_down} />
				<span><b>Add its Release to Turned down</b><br /><span class="hint">HolyArrs never picks it again for {manyWords(target.kind) ? `these ${manyWords(target.kind)?.many}` : 'this movie'}.</span></span>
			</label>
			<label class="checkline" class:off={!choice.turn_down}>
				<input type="checkbox" bind:checked={choice.search_again} disabled={!choice.turn_down} />
				<span><b>Search again</b><br /><span class="hint">Look for the next Release straight away. Without it the {manyWords(target.kind) ? `${manyWords(target.kind)?.many} wait` : 'movie waits'} for the next feed check.</span></span>
			</label>
			{#if removeError}<p class="form-error" role="alert">{removeError}</p>{/if}
			<div class="row modal-actions">
				<button class="btn btn-quiet" type="button" onclick={() => removeDialog?.close()}>Cancel</button>
				<button class="btn btn-red" type="submit" disabled={busy !== 0} aria-busy={busy !== 0}>Remove</button>
			</div>
		</form>
	{/if}
</dialog>

<style>
	.page-actions {
		display: flex;
		align-items: center;
		gap: var(--s-2);
		flex-wrap: wrap;
	}
	.eyebrow {
		margin: 0 0 var(--s-2);
		font-size: 11px;
		font-weight: var(--weight-semibold);
		letter-spacing: 0.09em;
		text-transform: uppercase;
		color: var(--text-muted);
	}
	.loading-grid,
	.download-workspace {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(260px, 320px);
		gap: var(--s-4);
		align-items: start;
		max-width: 1600px;
	}
	.download-main {
		min-width: 0;
	}
	.queue {
		padding: 0;
		overflow: hidden;
		box-shadow: none;
	}
	.queue + .queue {
		margin-top: var(--s-4);
	}
	.queue .card-head {
		min-height: 56px;
		padding: var(--s-3) var(--s-4);
		margin: 0;
		border-bottom: 1px solid var(--line);
	}
	.count {
		display: grid;
		place-items: center;
		min-width: 24px;
		height: 24px;
		padding: 0 7px;
		border: 1px solid var(--line);
		border-radius: var(--r-sm);
		font: var(--weight-medium) var(--text-xs) / 1 var(--font-mono);
		color: var(--text-muted);
	}
	.empty-line {
		padding: var(--s-4);
	}
	.dl {
		align-items: flex-start;
		flex-wrap: wrap;
		padding: var(--s-3) var(--s-4);
		border-left: 3px solid transparent;
		transition: background var(--dur) var(--ease-out), border-color var(--dur) var(--ease-out);
	}
	.media-art {
		flex: 0 0 44px;
		width: 44px;
		--poster-ratio: 2 / 3;
	}
	.media-art.square,
	.detail-art.square {
		--poster-ratio: 1;
	}
	.dl.selected {
		background: var(--surface-2);
		border-left-color: var(--text);
	}
	.dl .top {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: var(--s-3);
		margin-bottom: var(--s-2);
	}
	.names {
		display: grid;
		min-width: 0;
	}
	.nm {
		font-weight: var(--weight-medium);
	}
	.meta {
		display: flex;
		flex-wrap: wrap;
		gap: var(--s-3);
		margin-top: 6px;
	}
	.txt {
		margin-top: 6px;
	}
	.covers {
		color: var(--text);
	}
	.eps {
		margin-top: var(--s-2);
	}
	.eps summary {
		cursor: pointer;
		min-height: 32px;
		display: flex;
		align-items: center;
	}
	.eplist {
		list-style: none;
		padding: 0;
		margin: var(--s-2) 0 0;
		display: grid;
		gap: var(--s-1);
	}
	.eplist li {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto;
		align-items: center;
		gap: var(--s-2);
	}
	.acts {
		display: flex;
		gap: var(--s-2);
		flex-wrap: wrap;
	}
	.inspect[aria-pressed='true'] {
		color: var(--text);
		background: var(--surface-3);
	}
	.imports li {
		padding-inline: var(--s-4);
	}
	.detail {
		position: sticky;
		top: calc(var(--topbar) + var(--s-4));
		min-width: 0;
		box-shadow: none;
	}
	.detail-art {
		width: 76px;
		margin-bottom: var(--s-4);
		--poster-ratio: 2 / 3;
	}
	.detail-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--s-2);
		margin-bottom: var(--s-3);
	}
	.detail-head .eyebrow {
		margin: 0;
	}
	.detail-title {
		overflow-wrap: anywhere;
		scroll-margin-top: calc(var(--topbar) + var(--s-4));
	}
	.detail-title:focus-visible {
		outline: 2px solid var(--info);
		outline-offset: 4px;
		border-radius: var(--r-sm);
	}
	.detail-path {
		margin-top: var(--s-3);
		overflow-wrap: anywhere;
	}
	.detail-facts {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto;
		gap: var(--s-2) var(--s-3);
		margin-top: var(--s-5);
		padding-top: var(--s-4);
		border-top: 1px solid var(--line);
		font-size: var(--text-sm);
	}
	.detail-facts dt {
		color: var(--text-muted);
	}
	.detail-facts dd {
		margin: 0;
		text-align: right;
		font-variant-numeric: tabular-nums;
		overflow-wrap: anywhere;
	}
	.detail-note {
		margin-top: var(--s-4);
		padding: var(--s-3);
		border: 1px solid var(--line);
		border-radius: var(--r-sm);
		background: var(--surface-2);
		overflow-wrap: anywhere;
	}
	.detail-actions {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--s-2);
		margin-top: var(--s-4);
	}
	.load-error {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--s-4);
	}
	.path {
		overflow-wrap: anywhere;
		margin-bottom: var(--s-4);
	}
	.off {
		opacity: 0.6;
	}
	.modal {
		width: min(480px, calc(100vw - 32px));
		max-height: calc(100dvh - 32px);
		overflow: auto;
		padding: var(--s-6);
		border: 0;
		border-radius: var(--r-modal);
		background: var(--surface-1);
		color: var(--text);
		box-shadow: var(--shadow-modal);
	}
	.modal::backdrop {
		background: var(--scrim);
	}
	.modal-actions {
		justify-content: flex-end;
		margin-top: var(--s-6);
	}
	@media (max-width: 980px) {
		.loading-grid,
		.download-workspace {
			grid-template-columns: 1fr;
		}
		.detail {
			position: static;
			grid-row: 1;
		}
	}
	@media (max-width: 620px) {
		.page-actions,
		.page-actions :global(.btn) {
			width: 100%;
		}
		.page-actions :global(.btn) {
			min-height: 44px;
		}
		.dl {
			padding: var(--s-3);
		}
		.media-art {
			flex-basis: 40px;
			width: 40px;
		}
		.acts,
		.acts :global(.btn) {
			width: 100%;
		}
		.acts :global(.btn) {
			min-height: 44px;
		}
		.load-error {
			align-items: stretch;
			flex-direction: column;
		}
		.detail-actions {
			grid-template-columns: 1fr;
		}
	}
</style>
