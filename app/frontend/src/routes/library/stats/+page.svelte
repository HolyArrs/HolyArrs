<script lang="ts">
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import Popover from '#lib/ui/Popover.svelte';
	import { onMount } from 'svelte';
	import { to } from '#lib/core/paths.ts';
	import { api, message } from '#lib/core/api.ts';
	import { bytes, ago } from '#lib/core/format.ts';
	import { plainReason } from '#lib/core/plain.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { LibraryToolPage, statFacts as factOrder, statValue as valueLabel, statUnit as unitWord, type KindStats, type Watched } from '#lib/mediafiles/index.ts';

	// Library stats: what the files are (Your files) and who watched what on the media servers.

	type Tab = 'files' | 'watched';
	let tab = $state<Tab>('files');

	let kinds = $state<KindStats[]>([]);
	let kind = $state('');
	let loaded = $state(false);
	let error = $state('');
	const shown = $derived(kinds.find((k) => k.kind === kind) ?? kinds[0]);
	const unchecked = $derived(shown?.facts.resolution?.find((b) => b.value === 'unchecked')?.files ?? shown?.facts.container?.find((b) => b.value === 'unchecked')?.files ?? 0);

	async function load() {
		try {
			kinds = (await api.get<{ kinds: KindStats[] }>('/library/stats')).kinds ?? [];
			if (!kinds.some((k) => k.kind === kind)) kind = kinds[0]?.kind ?? '';
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}

	let watched = $state<Watched | null>(null);
	let wError = $state('');
	async function loadWatched() {
		try {
			watched = await api.get<Watched>('/library/stats/watched');
			wError = '';
		} catch (e) {
			wError = message(e);
		}
	}
	onMount(() => {
		if (new URLSearchParams(location.search).get('tab') === 'watched') tab = 'watched';
		void load();
		void loadWatched();
	});
	function pickTab(v: Tab) {
		tab = v;
		const u = new URL(location.href);
		if (v === 'watched') u.searchParams.set('tab', 'watched');
		else u.searchParams.delete('tab');
		history.replaceState(history.state, '', u);
	}
	const isOwner = $derived(session.s.role === 'owner');
</script>

<svelte:head><title>{t('library.stats.title')} · HolyArrs</title></svelte:head>

<LibraryToolPage title={t('library.stats.title')}>
	{#snippet bar()}
		<span class="tp-gap"></span>
		<SegmentedControl size="sm" label={t('library.stats.title')} options={[{ value: 'files', label: t('library.stats.files') }, { value: 'watched', label: t('library.stats.watched') }] as { value: Tab; label: string }[]} value={tab} onchange={pickTab} testid="stats-tab" />
	{/snippet}

{#if tab === 'files'}
	{#if error}
		<LoadError hasData={false} message={plainReason(error).text} onretry={load} />
	{:else}
	<PageLoader loading={!loaded} label={t('library.stats.loading')}>
	{#if !shown || shown.files === 0 && kinds.every((k) => k.files === 0)}
		<EmptyState icon="gauge" title={t('library.stats.empty')} text={t('library.stats.emptyText')} />
	{:else}
		{#if kinds.length > 1}
			<SegmentedControl size="sm" label={t('library.stats.kind')} options={kinds.map((k) => ({ value: k.kind, label: t('library.stats.kind.' + k.kind) }))} bind:value={kind} testid="stats-kind" />
		{/if}
		<div class="tiles" data-testid="stats-totals">
			<div class="tp-sheet tile"><span class="lab">{t('library.stats.titles')}</span><b>{shown.titles.toLocaleString()}</b></div>
			<div class="tp-sheet tile"><span class="lab">{t('library.stats.fileCount')}</span><b>{shown.files.toLocaleString()}</b></div>
			<div class="tp-sheet tile"><span class="lab">{t('library.stats.onDisk')}</span><b>{bytes(shown.bytes)}</b></div>
			<div class="tp-sheet tile"><span class="lab">{t('library.stats.value.unchecked')}</span><b>{unchecked.toLocaleString()}</b></div>
		</div>
		<div class="grid">
			{#each factOrder.filter((f) => shown.facts[f]?.length) as f (f)}
				<section class="tp-sheet fact" data-testid="stats-fact">
					<h2 class="fh">{t('library.stats.fact.' + f)}</h2>
					<ul>
						{#each shown.facts[f] as b (b.value)}
							<li>
								{#if b.value === 'unknown'}
									<span class="nm hinted"><span class="tx">{valueLabel(f, b.value, t)}</span><Popover label={t('library.stats.value.unknown')} icon="info" width={260} portal><p class="hint">{t('library.stats.value.unknownHint')}</p></Popover></span>
								{:else}
									<span class="nm">{valueLabel(f, b.value, t)}</span>
								{/if}
								<span class="bar" aria-hidden="true"><i style="transform: scaleX({shown.files ? b.files / shown.files : 0})"></i></span>
								<span class="num">{b.files.toLocaleString()} · {bytes(b.bytes)}</span>
							</li>
						{/each}
					</ul>
				</section>
			{/each}
		</div>
	{/if}
	</PageLoader>
	{/if}
{:else}
	{#if wError && !watched}
		<LoadError hasData={false} message={plainReason(wError).text} onretry={loadWatched} />
	{:else}
	<PageLoader loading={!watched} label={t('library.stats.loading')}>
	{#if watched && !watched.enabled}
		<EmptyState icon="eye" title={t('library.stats.offTitle')} text={isOwner ? t('library.stats.offOwner') : t('library.stats.offText')} action={isOwner ? { label: t('library.stats.openMediaServers'), href: to('/settings/media-servers') } : undefined} />
	{:else if watched && !watched.read_at}
		<EmptyState icon="clock" title={t('library.stats.firstTitle')} text={t('library.stats.firstText')} />
	{:else if watched}
		<p class="help">{t('library.stats.watchedHelp')}{#if watched.people?.length} {t('library.stats.lastRead', { ago: ago(watched.read_at) })}{/if}</p>
		{#if !watched.people?.length}
			<EmptyState icon="eye" title={t('library.stats.noMatch', { ago: ago(watched.read_at) })} compact />
		{:else}
		<div class="tp-sheet">
			<ul class="tp-rows people">
				{#each watched.people as p (p.server + '/' + p.name)}
					<li class="person" data-testid="stats-person">
						<span class="av" aria-hidden="true">{(p.user_name || p.name).slice(0, 1).toUpperCase()}</span>
						<div class="who">
							<span class="name">{p.linked && p.user_name ? p.user_name : p.name}</span>
							<span class="tp-sub">{p.server}{p.linked && p.user_name && p.user_name !== p.name ? ` · ${p.name}` : ''}{#if !p.linked} <span class="chip">{t('library.stats.notLinked')}</span>{/if}</span>
						</div>
						<span class="counts">
							<span>{unitWord('movie', p.movies, t)}</span>
							<span>{unitWord('episode', p.episodes, t)}</span>
							<span>{unitWord('play', p.plays, t)}</span>
							{#if p.last_watched_at}<span class="muted">{t('library.stats.lastWatched', { ago: ago(p.last_watched_at) })}</span>{/if}
						</span>
					</li>
				{/each}
			</ul>
		</div>
		{/if}
	{/if}
	</PageLoader>
	{/if}
{/if}
</LibraryToolPage>

<style>
	.tiles { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
	.tile { display: flex; flex-direction: column; gap: 6px; padding: 16px 20px; }
	.lab { color: var(--text-muted); font: var(--t-footnote); }
	.tile b { align-self: flex-start; max-width: 100%; font: 600 24px/1.1 var(--font-display, var(--font-ui)); font-variant-numeric: tabular-nums; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
	.fact { padding: 16px 20px; container-type: inline-size; }
	.fh { margin: 0; font: var(--t-headline); }
	.fact ul { display: grid; grid-template-columns: minmax(0, 9rem) minmax(3rem, 1fr) auto; gap: 10px 12px; margin: 12px 0 0; padding: 0; list-style: none; }
	.fact li { grid-column: 1 / -1; display: grid; grid-template-columns: subgrid; align-items: center; font: var(--t-callout); }
	@container (max-width: 440px) {
		.fact ul { grid-template-columns: minmax(0, 1fr) auto; }
		.fact li { row-gap: 6px; }
		.fact li .bar { grid-column: 1 / -1; order: 3; }
	}
	.nm { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.nm.hinted { display: flex; align-items: center; gap: 2px; overflow: visible; }
	.nm.hinted .tx { min-width: 0; overflow: hidden; text-overflow: ellipsis; }
	/* The info button keeps its full tap area but does not make its row taller than the others. */
	.nm.hinted :global(.trigger) { flex: none; margin-block: -8px; }
	@media (pointer: coarse) { .nm.hinted :global(.trigger) { margin-block: -12px; } }
	.hint { margin: 0; padding: 4px 2px; font: var(--t-callout); }
	.bar { height: 6px; border-radius: 3px; background: color-mix(in srgb, var(--text) 8%, transparent); overflow: hidden; }
	.bar i { display: block; height: 100%; background: var(--accent); transform-origin: left; transition: transform var(--dur-slow) var(--ease-out); }
	:global([dir='rtl']) .bar i { transform-origin: right; }
	.num { justify-self: end; color: var(--text-muted); white-space: nowrap; font-variant-numeric: tabular-nums; }
	.num, .tile b { direction: ltr; unicode-bidi: isolate; }
	.help { margin: 0; color: var(--text-muted); font: var(--t-callout); max-width: 70ch; }
	.person { display: flex; align-items: center; gap: 12px; }
	.av { flex: none; display: grid; place-items: center; width: 36px; height: 36px; border-radius: 50%; background: var(--surface-2); font-weight: 600; }
	.who { flex: 1; min-width: 0; display: flex; flex-direction: column; font: var(--t-callout); }
	.name { font-weight: 600; overflow-wrap: anywhere; }
	.counts { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 4px 14px; font: var(--t-footnote); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.muted { color: var(--text-muted); }
	@media (prefers-reduced-motion: reduce) { .bar i { transition: none; } }
	@media (max-width: 639px) {
		.tiles { grid-template-columns: 1fr 1fr; }
		.grid { grid-template-columns: 1fr; }
		.tile { padding: 14px 16px; }
		.tile b { font-size: 20px; }
		.fact { padding: 14px 16px; }
		.person { flex-wrap: wrap; align-items: flex-start; }
		.counts { flex-basis: 100%; justify-content: flex-start; padding-inline-start: 48px; }
	}
</style>
