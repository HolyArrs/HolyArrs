<script lang="ts">
	import { to } from '#lib/paths.ts';
	import { onMount, tick } from 'svelte';
	import { api, message } from '#lib/api.ts';
	import { live } from '#lib/live.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { ago, bytes, duration, percent, tone, words } from '#lib/format.ts';
	import { episodeStateWords, manyWords, phaseTone, removeBody, removeStart, rowName, split, type DownloaderSettings, type DownloadRow, type RemoveChoice } from '#lib/downloaders.ts';
	import Poster from '#lib/Poster.svelte';
	import { session } from '#lib/session.svelte.ts';
	import { plainReason } from '#lib/plain.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import OpsDownloadsTabs from '#lib/ui/OpsDownloadsTabs.svelte';
	import Button from '#lib/ui/Button.svelte';
	import StatusPill, { type StatusKind } from '#lib/ui/StatusPill.svelte';
	import ProgressBar from '#lib/ui/ProgressBar.svelte';
	import ListRow from '#lib/ui/ListRow.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Menu from '#lib/ui/Menu.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
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

	// Wide screens show the selected download in a side panel; narrower ones
	// keep the list first and open the details in a Sheet.
	let wide = $state(true);
	let detailOpen = $state(false);
	async function selectDownload(id: number) {
		selectedId = id;
		if (!wide) {
			detailOpen = true;
			return;
		}
		await tick();
		detailHeading?.focus({ preventScroll: true });
	}

	/** toneKind picks the kit status colour for a tone; the word stays the row's own. */
	function toneKind(t: string): StatusKind {
		return t === 'ok' ? 'in_library' : t === 'fail' ? 'failed' : t === 'idle' ? 'turned_down' : 'missing';
	}
	function pill(r: DownloadRow): StatusKind {
		return r.phase === 'downloading' ? 'downloading' : toneKind(phaseTone(r.phase));
	}
	const moving = (r: DownloadRow) => r.phase !== 'removed' && r.phase !== 'failed' && r.phase !== 'vanished';
	const admin = $derived(session.can('settings'));

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
		const mq = window.matchMedia('(min-width: 1024px)');
		const fit = () => {
			wide = mq.matches;
			if (wide) detailOpen = false;
		};
		fit();
		mq.addEventListener('change', fit);
		void load();
		void api.get<{ titles: TitleView[] }>('/titles').then((r) => (titles = r.titles)).catch(() => {});
		api.get<DownloaderSettings>('/settings/downloaders').then((s) => (settings = s)).catch(() => {});
		const offs = ['download', 'slot', 'import', 'resync'].map((t) => live.on(t, soon));
		return () => {
			offs.forEach((off) => off());
			mq.removeEventListener('change', fit);
			clearTimeout(timer);
		};
	});

	function who(r: DownloadRow) {
		const n = rowName(r);
		return manyWords(r.kind) && r.covers ? `${n} ${r.covers}` : n;
	}

	let busy = $state(0);
	async function markFailed(r: DownloadRow) {
		detailOpen = false;
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
	let removeOpen = $state(false);
	let target = $state<DownloadRow | null>(null);
	let choice = $state<RemoveChoice>(removeStart(null));
	let removeError = $state('');
	function askRemove(r: DownloadRow) {
		target = r;
		choice = removeStart(settings);
		removeError = '';
		detailOpen = false;
		removeOpen = true;
	}
	async function remove(e: SubmitEvent) {
		e.preventDefault();
		if (!target) return;
		busy = target.id;
		try {
			await api.post(`/downloads/${target.id}/remove`, removeBody(choice));
			removeOpen = false;
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

<PageHeader title="Downloads" subtitle="What your Downloaders are fetching right now, and what HolyArrs moved into the Library." />
<OpsDownloadsTabs />

{#snippet note(r: DownloadRow)}
	{#if r.text}
		{@const p = plainReason(r.text)}
		<div class="note" class:bad={phaseTone(r.phase) === 'fail'}>
			<p data-testid="row-text">{p.text}</p>
			{#if p.detail && admin}<details class="raw"><summary>Details</summary><p>{p.detail}</p></details>{/if}
		</div>
	{/if}
{/snippet}

{#snippet dlRow(r: DownloadRow)}
	{@const pct = percent(r.progress)}
	<li class="dl" class:selected={wide && selected?.id === r.id} data-testid="download-row" data-phase={r.phase}>
		<span class="art" class:square={r.kind === 'artist'} aria-hidden="true"><Poster src={r.artwork ?? titleById.get(r.title_id ?? 0)?.artwork[0]} name={r.title || r.name} /></span>
		<div class="body">
			<div class="top">
				<div class="names">
					{#if r.title_id}
						<a class="nm" href={to(`/library/${r.title_id}`)}>{rowName(r)}</a>
					{:else}
						<span class="nm">{rowName(r)}</span>
					{/if}
					{#if manyWords(r.kind) && r.covers}<span class="covers" data-testid="row-covers">{r.covers}</span>{/if}
					<span class="rel" title={r.name}>{r.name}</span>
				</div>
				<StatusPill kind={pill(r)} label={r.label} testid="row-state" />
			</div>
			{#if moving(r)}
				<div class="prog"><ProgressBar value={pct} label="Progress of {who(r)}" size="sm" tone={r.progress >= 1 ? 'ok' : 'info'} /></div>
			{/if}
			<p class="meta">
				<span data-testid="row-percent">{pct}%</span>
				{#if r.active && r.eta}<span data-testid="row-eta">{duration(r.eta)} left</span>{/if}
				{#if r.active && r.speed}<span data-testid="row-speed">{bytes(r.speed)}/s</span>{/if}
				{#if r.size}<span>{bytes(r.size)}</span>{/if}
				{#if !r.active}<span>{ago(r.updated_at)}</span>{/if}
			</p>
			{@render note(r)}
			{#if manyWords(r.kind) && r.episodes && r.episodes.length > 1}
				<details class="eps" data-testid="row-episodes">
					<summary>The {r.episodes.length} {manyWords(r.kind)?.many} it covers</summary>
					<ul class="eplist">
						{#each r.episodes as e (e.slot_id)}
							{@const st = episodeStateWords(e.state)}
							<li data-testid="row-episode">
								<span class="eplabel">{e.label}</span>
								<span class="epname">{e.name ?? ''}</span>
								<StatusPill kind={toneKind(st.tone)} label={st.text} />
							</li>
						{/each}
					</ul>
				</details>
			{/if}
		</div>
		<div class="acts">
			<Button variant={wide && selected?.id === r.id ? 'secondary' : 'plain'} aria-label="Details for {who(r)}" aria-pressed={wide ? selected?.id === r.id : undefined} onclick={() => void selectDownload(r.id)}>Details</Button>
			{#if r.can_act}
				<Menu
					label="More for {who(r)}"
					items={[
						{ label: 'Mark failed', icon: 'alert', disabled: busy === r.id, onselect: () => markFailed(r) },
						{ label: 'Remove', icon: 'trash', destructive: true, disabled: busy === r.id, onselect: () => askRemove(r) }
					]}
				/>
			{/if}
		</div>
	</li>
{/snippet}

{#snippet detail(s: DownloadRow)}
	{@const p = s.text ? plainReason(s.text) : null}
	<div class="d-head">
		<span class="d-art" class:square={s.kind === 'artist'} aria-hidden="true"><Poster src={s.artwork ?? titleById.get(s.title_id ?? 0)?.artwork[0]} name={s.title || s.name} /></span>
		<div class="d-names">
			{#if wide}<h2 class="d-title" bind:this={detailHeading} tabindex="-1">{rowName(s)}</h2>{:else}<p class="d-title">{rowName(s)}</p>{/if}
			{#if manyWords(s.kind) && s.covers}<p class="d-sub">{s.covers}</p>{/if}
			<StatusPill kind={pill(s)} label={s.label} />
		</div>
	</div>
	{#if moving(s)}<ProgressBar value={percent(s.progress)} label="Progress of {who(s)}" showValue tone={s.progress >= 1 ? 'ok' : 'info'} />{/if}
	<dl class="facts">
		<dt>Progress</dt><dd>{percent(s.progress)}%</dd>
		{#if s.active && s.eta}<dt>Time left</dt><dd>{duration(s.eta)}</dd>{/if}
		{#if s.active && s.speed}<dt>Speed</dt><dd>{bytes(s.speed)}/s</dd>{/if}
		{#if s.size}<dt>Size</dt><dd>{bytes(s.size)}</dd>{/if}
		{#if s.downloader}<dt>Downloader</dt><dd>{s.downloader}</dd>{/if}
		{#if s.version}<dt>Version</dt><dd>{s.version}</dd>{/if}
	</dl>
	{#if p}<div class="note" class:bad={phaseTone(s.phase) === 'fail'}><p>{p.text}</p></div>{/if}
	<details class="raw">
		<summary>Details</summary>
		<p>{s.name}</p>
		{#if admin && p?.detail}<p>{p.detail}</p>{/if}
	</details>
	{#if s.can_act}
		<div class="d-acts">
			<Button variant="secondary" full onclick={() => markFailed(s)} disabled={busy === s.id}>Mark failed</Button>
			<Button variant="destructive" full onclick={() => askRemove(s)} disabled={busy === s.id}>Remove</Button>
		</div>
	{/if}
{/snippet}

{#if !loaded}
	<div class="loading" aria-label="Loading downloads">
		<Skeleton shape="block" height="96px" /><Skeleton shape="block" height="96px" /><Skeleton shape="block" height="96px" />
	</div>
{:else if error && rows.length === 0 && imports.length === 0}
	<LoadError hasData={false} message="Downloads could not be loaded." onretry={load} />
{:else if rows.length === 0 && imports.length === 0}
	<EmptyState icon="download" title="No downloads yet" text="When HolyArrs chooses a Release for a movie or an episode, it sends it to a Downloader and you can watch it here, live." action={{ label: 'Add media', href: to('/search'), icon: 'search' }} />
	<p class="aside-link"><a href={to('/settings/downloaders')}>Downloaders</a></p>
{:else}
	{#if error}<LoadError hasData message="Downloads could not be loaded." onretry={load} />{/if}
	<div class="work" class:wide>
		<div class="lists">
			<section aria-labelledby="h-d">
				<header class="sec"><h2 id="h-d">Now</h2><span class="count">{parts.now.length}</span></header>
				{#if parts.now.length === 0}
					<p class="panel empty-line">Nothing is downloading.</p>
				{:else}
					<ul class="panel rows">{#each parts.now as r (r.id)}{@render dlRow(r)}{/each}</ul>
				{/if}
			</section>

			{#if parts.past.length}
				<section aria-labelledby="h-p">
					<header class="sec"><h2 id="h-p">Finished and stopped</h2><span class="count">{parts.past.length}</span></header>
					<ul class="panel rows">{#each parts.past as r (r.id)}{@render dlRow(r)}{/each}</ul>
				</section>
			{/if}

			<section aria-labelledby="h-i">
				<header class="sec"><h2 id="h-i">Imports</h2><span class="count">{imports.length}</span></header>
				{#if imports.length === 0}
					<p class="panel empty-line">No imports yet. A finished download is moved into the Library here.</p>
				{:else}
					<ul class="panel imports">
						{#each imports as im (im.ID)}
							{@const placed = im.Result?.find((r) => r.outcome === 'placed')}
							<li>
								<ListRow icon="folder" title={placed?.source_file ?? im.Plan?.[0]?.source_file ?? `Import ${im.ID}`} subtitle={placed?.placed_by ? `Placed by ${placed.placed_by}` : undefined} meta={ago(im.CreatedAt)}>
									{#snippet trailing()}<StatusPill kind={toneKind(tone(im.State))} label={words(im.State)} />{/snippet}
								</ListRow>
							</li>
						{/each}
					</ul>
				{/if}
			</section>
		</div>

		{#if wide}
			<aside class="side panel" aria-label="Selected download details">
				{#if selected}{@render detail(selected)}{:else}<p class="muted">Choose a download to see its details.</p>{/if}
			</aside>
		{/if}
	</div>
{/if}

{#if !wide}
	<Sheet bind:open={detailOpen} title="Download details" size="md">
		{#if selected}{@render detail(selected)}{/if}
	</Sheet>
{/if}

<Sheet bind:open={removeOpen} title="Remove this download?" size="sm">
	{#if target}
		<form id="remove-form" onsubmit={remove}>
			<p class="who">{who(target)}</p>
			<details class="raw"><summary>Details</summary><p>{target.name}</p></details>
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
		</form>
	{/if}
	{#snippet footer()}
		<Button variant="plain" onclick={() => (removeOpen = false)}>Cancel</Button>
		<Button variant="destructive" type="submit" form="remove-form" busy={busy !== 0} disabled={busy !== 0}>Remove</Button>
	{/snippet}
</Sheet>

<style>
	.loading { display: grid; gap: 12px; max-width: 980px; }
	.aside-link { text-align: center; margin-top: -24px; }
	.aside-link a { display: inline-flex; align-items: center; min-height: 44px; color: var(--accent-text); font: var(--t-callout); font-weight: 500; }
	.work { display: grid; gap: 24px; align-items: start; }
	.work.wide { grid-template-columns: minmax(0, 1fr) minmax(280px, 340px); }
	.lists { display: flex; flex-direction: column; gap: 28px; min-width: 0; }
	.sec { display: flex; align-items: center; gap: 10px; margin-bottom: 8px; }
	.sec h2 { font: var(--t-title3); letter-spacing: var(--t-title3-track); }
	.count { min-width: 24px; height: 24px; padding: 0 8px; display: inline-grid; place-items: center; border-radius: 999px; background: var(--surface-2); color: var(--text-muted); font: var(--t-subhead); font-weight: 600; font-variant-numeric: tabular-nums; }
	.panel { margin: 0; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); }
	.rows, .imports { list-style: none; padding: 6px; }
	.empty-line { padding: 16px; color: var(--text-muted); font: var(--t-callout); }
	.dl { display: grid; grid-template-columns: 44px minmax(0, 1fr) auto; gap: 14px; padding: 12px 10px; border-radius: var(--r-btn); transition: background var(--dur-fast) var(--ease-out); }
	.dl + .dl { box-shadow: 0 -1px 0 var(--line); }
	.dl.selected { background: var(--surface-2); box-shadow: inset 3px 0 0 var(--accent); }
	.art { width: 44px; --poster-ratio: 2 / 3; }
	.art.square, .d-art.square { --poster-ratio: 1; }
	.body { min-width: 0; display: flex; flex-direction: column; gap: 6px; }
	.top { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
	.names { display: flex; flex-direction: column; min-width: 0; }
	.nm { color: var(--text); font: var(--t-headline); font-size: 15px; overflow-wrap: anywhere; }
	a.nm:hover { text-decoration: underline; text-underline-offset: 3px; }
	.covers { font: var(--t-callout); color: var(--text); }
	.rel { color: var(--text-muted); font: var(--t-footnote); font-weight: 400; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.prog { max-width: 520px; }
	.meta { display: flex; flex-wrap: wrap; gap: 4px 14px; color: var(--text-muted); font: var(--t-subhead); font-weight: 400; font-variant-numeric: tabular-nums; }
	.note { padding: 8px 12px; border-radius: var(--r-btn); background: var(--surface-2); font: var(--t-callout); overflow-wrap: anywhere; }
	.note.bad { box-shadow: inset 3px 0 0 var(--fail); }
	.raw { color: var(--text-muted); font: var(--t-footnote); }
	.raw summary { width: max-content; min-height: 24px; cursor: pointer; border-radius: 6px; }
	.raw summary:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.raw p { margin-top: 4px; font-family: var(--font-mono); overflow-wrap: anywhere; }
	.note .raw { margin-top: 4px; }
	.eps summary { width: max-content; max-width: 100%; min-height: 32px; display: flex; align-items: center; cursor: pointer; color: var(--accent-text); font: var(--t-subhead); border-radius: 6px; }
	.eps summary:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.eplist { list-style: none; padding: 0; margin: 6px 0 0; display: grid; gap: 6px; }
	.eplist li { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 10px; font: var(--t-callout); }
	.eplabel { color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.epname { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.acts { display: flex; align-items: center; gap: 4px; }
	.acts > :global(.ui-btn) { color: var(--text-muted); font-weight: 500; }
	.acts > :global(.ui-btn:hover) { color: var(--text); }
	.side { position: sticky; top: calc(var(--topbar, 56px) + 16px); padding: 18px; display: flex; flex-direction: column; gap: 14px; min-width: 0; }
	.side .muted { color: var(--text-muted); font: var(--t-callout); }
	.d-head { display: flex; gap: 14px; align-items: flex-start; }
	.d-art { flex: none; width: 64px; --poster-ratio: 2 / 3; }
	.d-names { display: flex; flex-direction: column; align-items: flex-start; gap: 6px; min-width: 0; }
	.d-title { font: var(--t-title3); overflow-wrap: anywhere; border-radius: 6px; }
	.d-title:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.d-sub { font: var(--t-callout); color: var(--text-muted); }
	.facts { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 8px 12px; margin: 0; padding-top: 12px; border-top: 1px solid var(--line); font: var(--t-callout); }
	.facts dt { color: var(--text-muted); }
	.facts dd { margin: 0; font-family: inherit; text-align: right; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
	.d-acts { display: grid; gap: 8px; }
	.who { font: var(--t-headline); margin-bottom: 4px; }
	form { display: flex; flex-direction: column; gap: 14px; }
	.checkline { display: flex; gap: 12px; align-items: flex-start; font: var(--t-callout); cursor: pointer; }
	.checkline input { width: 20px; height: 20px; margin-top: 1px; flex: none; accent-color: var(--accent); }
	.hint { color: var(--text-muted); font: var(--t-subhead); font-weight: 400; }
	.off { opacity: 0.6; }
	.form-error { color: var(--fail); font: var(--t-callout); }
	@media (max-width: 640px) {
		.dl { grid-template-columns: 40px minmax(0, 1fr); gap: 12px; }
		.art { width: 40px; }
		.top { flex-direction: column; align-items: stretch; gap: 6px; }
		.top > :global(.ui-pill) { align-self: flex-start; }
		.acts { grid-column: 1 / -1; justify-content: space-between; gap: 6px; }
		.acts > :global(.ui-btn) { flex: 1 1 auto; min-height: 44px; }
		.eplist li { grid-template-columns: auto minmax(0, 1fr); }
		.eplist li > :global(.ui-pill) { grid-column: 2; justify-self: start; }
		.epname { white-space: normal; overflow-wrap: anywhere; }
	}
</style>
