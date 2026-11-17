<!-- The scanned folders in the Library's four views: Rows, Banners, Posters and Overview. Each folder keeps its tick, title, folder, status and match picker. -->
<script lang="ts">
	import Poster from '#lib/ui/Poster.svelte';
	import { posterMinFor, posterRatio, type Density, type PosterSize } from '#lib/ui/library-art.ts';
	import BannerArt from './BannerArt.svelte';
	import ImportFolderFiles from './ImportFolderFiles.svelte';
	import { canAdopt, fileName, score, type AdoptCandidate } from './organise.ts';
	import { bannerTitle, matchOptions, needsLook, pickedOf, statusLabel, type ImportView, type Named } from './import-view.ts';
	import { bytes } from '../core/format.ts';
	import { to } from '../core/paths.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	let {
		folders,
		view,
		size,
		density,
		kind,
		phone,
		sel = $bindable(),
		ticked = $bindable(),
		named,
		onlook,
		onzoom
	}: {
		folders: AdoptCandidate[];
		view: ImportView;
		size: PosterSize;
		density: Density;
		kind: 'movie' | 'show' | 'music';
		phone: boolean;
		sel: Record<string, string>;
		ticked: Record<string, boolean>;
		named: Record<string, Named>;
		onlook: (f: AdoptCandidate) => void;
		onzoom: (f: AdoptCandidate) => void;
	} = $props();

	const LOOK = '\u0000look';
	const word = $derived(kind === 'show' ? 'show' : kind === 'music' ? 'artist' : 'movie');
	const w = $derived(t(`library.import.word.${word}`));
	const artKind = $derived(kind === 'music' ? 'artist' : kind);
	const bannerW = $derived({ large: 380, comfortable: 300, compact: 220 }[density] ?? 380);

	// Art and pickers mount only for folders near the screen, so a scan of thousands stays fast.
	let near = $state<Record<string, boolean>>({});
	let seen: IntersectionObserver | undefined;
	function whenNear(node: HTMLElement, folder: string) {
		if (typeof IntersectionObserver === 'undefined') {
			near[folder] = true;
			return {};
		}
		seen ??= new IntersectionObserver(
			(es) => {
				for (const e of es) {
					if (!e.isIntersecting) continue;
					const el = e.target as HTMLElement;
					near[el.dataset.folder ?? ''] = true;
					seen?.unobserve(el);
				}
			},
			{ rootMargin: '600px 0px' }
		);
		node.dataset.folder = folder;
		seen.observe(node);
		return { destroy: () => seen?.unobserve(node) };
	}

	function choose(f: AdoptCandidate, el: HTMLSelectElement) {
		if (el.value === LOOK) {
			el.value = sel[f.folder] ?? '';
			onlook(f);
			return;
		}
		sel[f.folder] = el.value;
		ticked[f.folder] = !!el.value;
	}

	function pillText(f: AdoptCandidate): string {
		const label = statusLabel(f.status, kind);
		return (f.status === 'matched' || f.status === 'unsure') && f.match ? `${label} · ${score(f.match.score)}` : label;
	}
</script>

{#snippet tickBox(f: AdoptCandidate)}
	{#if canAdopt(f)}
		<input class="tk" type="checkbox" bind:checked={ticked[f.folder]} disabled={!sel[f.folder]} aria-label={t('library.import.adoptFolder', { folder: f.folder })} />
	{:else}
		<span class="tk" aria-hidden="true"></span>
	{/if}
{/snippet}

{#snippet pill(f: AdoptCandidate)}
	<span class="pill {f.status === 'matched' ? 'ok' : needsLook(f) ? 'un' : 'off'}" title={f.why || undefined}>{pillText(f)}</span>
{/snippet}

{#snippet titleLine(f: AdoptCandidate, big = false)}
	{@const p = pickedOf(f, sel[f.folder], named)}
	<span class="tl" class:big title={`${p.name}${p.year ? ` (${p.year})` : ''}`}><b>{p.name}</b>{#if p.year}<small>{p.year}</small>{/if}</span>
{/snippet}

{#snippet art(f: AdoptCandidate, cls: string)}
	{@const p = pickedOf(f, sel[f.folder], named)}
	{#if p.poster}
		<button type="button" class="pos zoom {cls}" aria-label={t('library.import.seeLarge', { name: p.name })} onclick={() => onzoom(f)}>{#if near[f.folder]}<Poster src={p.poster} name={p.name} />{/if}</button>
	{:else}
		<span class="pos {cls}">{#if near[f.folder]}<Poster src="" name={p.name} />{/if}</span>
	{/if}
{/snippet}

{#snippet pick(f: AdoptCandidate)}
	{#if canAdopt(f)}
		{@const opts = matchOptions(f, sel[f.folder], named)}
		{@const now = opts.find((o) => o.value === sel[f.folder])?.label ?? t('library.import.nonePicked', { word: w })}
		{#if near[f.folder]}
			<select class="pk" class:un={needsLook(f) && !ticked[f.folder]} value={sel[f.folder] ?? ''} title={now} aria-label={t('library.import.matchFor', { folder: f.folder })} onchange={(e) => choose(f, e.currentTarget)} data-testid="import-pick">
				{#if !sel[f.folder]}<option value="" disabled>{t('library.import.nonePicked', { word: w })}</option>{/if}
				{#each opts as o (o.value)}<option value={o.value}>{o.label}</option>{/each}
				<option value={LOOK}>{t('library.import.notThis')}</option>
			</select>
		{:else}
			<span class="pk still">{now}</span>
		{/if}
	{:else if f.status === 'in_library' && f.title_id}
		<a class="small open" href={to(`/library/${f.title_id}`)}>{t('library.import.openThe', { word: w })}</a>
	{:else}
		<span class="why">{f.why}</span>
	{/if}
{/snippet}

{#snippet folderLine(f: AdoptCandidate)}<span class="fd" title={f.folder}>{f.folder}</span>{/snippet}

<div class="il v-{view}" class:phone style:--poster-ratio={posterRatio(artKind)} style:--pmin="{posterMinFor(size, artKind)}px" style:--bw="{bannerW}px" style:--bratio={kind === 'music' ? 2.5 : 5.405} data-testid="import-list" data-view={view}>
	{#if view === 'posters'}
		<ul class="grid sz-{size}">
			{#each folders as f (f.folder)}
				<li class="card" class:unr={needsLook(f) && !ticked[f.folder]} data-testid="import-item" data-status={f.status} use:whenNear={f.folder}>
					<div class="pw">
						{@render art(f, 'wall')}
						<label class="ovl">{@render tickBox(f)}</label>
						<span class="ovr">{@render pill(f)}</span>
					</div>
					{@render titleLine(f)}
					{#if size !== 's'}{@render folderLine(f)}{/if}
					{@render pick(f)}
					<ImportFolderFiles {f} />
				</li>
			{/each}
		</ul>
	{:else}
		<div class="sheet">
			{#if !phone && view !== 'overview'}
				<div class="hd row">
					<span></span>
					{#if view === 'rows'}<span></span><span>{t('mediafiles.list.title')}</span><span class="c-fd">{t('mediafiles.col.path')}</span>{:else}<span>{t('mediafiles.list.banner')}</span><span>{t('library.import.titleAndFolder')}</span>{/if}
					<span>{t('mediafiles.col.status')}</span>
					<span>{t('discover.leftovers.match')}</span>
				</div>
			{/if}
			{#each folders as f (f.folder)}
				{@const p = pickedOf(f, sel[f.folder], named)}
				<div class="item row" class:unr={needsLook(f) && !ticked[f.folder]} data-testid="import-item" data-status={f.status} use:whenNear={f.folder}>
					{#if view === 'rows'}
						{@render tickBox(f)}
						{@render art(f, 'thumb')}
						<span class="tx">{@render titleLine(f)}<span class="c-sub">{@render folderLine(f)}</span></span>
						<span class="c-fd">{@render folderLine(f)}</span>
						<span class="c-st">{@render pill(f)}</span>
						<span class="pkw">{@render pick(f)}</span>
						{#if f.files?.length || f.albums?.length}<span class="more"><ImportFolderFiles {f} /></span>{/if}
					{:else if view === 'banners'}
						{@render tickBox(f)}
						<span class="ban">{#if near[f.folder]}<BannerArt title={bannerTitle(kind, sel[f.folder], p)} width={phone ? 0 : bannerW} />{/if}</span>
						<span class="tx">
							{@render titleLine(f, true)}
							{@render folderLine(f)}
							{#if f.main}<span class="fd">{fileName(f.main)}</span>{/if}
							<span class="c-st2">{@render pill(f)}</span>
							<ImportFolderFiles {f} />
						</span>
						<span class="c-st">{@render pill(f)}</span>
						<span class="pkw">{@render pick(f)}</span>
					{:else}
						{@render tickBox(f)}
						{@render art(f, 'op')}
						<span class="tx">
							<span class="tline">{@render titleLine(f, true)}{@render pill(f)}</span>
							<dl>
								<dt>{t('mediafiles.col.path')}</dt><dd title={f.folder}>{f.folder}</dd>
								{#if f.main}<dt>{t('mediafiles.sheet.file')}</dt><dd title={fileName(f.main)}>{fileName(f.main)}</dd>{/if}
								{#if f.videos[0]}<dt>{t('mediafiles.col.size')}</dt><dd>{bytes(f.videos[0].size)}</dd>{/if}
							</dl>
							{#if f.why}<span class="why">{f.why}</span>{/if}
							<ImportFolderFiles {f} />
						</span>
						<span class="pkw"><span class="lab">{t('discover.leftovers.match')}</span>{@render pick(f)}</span>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>

<style>
	.il { --unbg: color-mix(in srgb, var(--wait) 7%, transparent); container-type: inline-size; min-width: 0; font: var(--t-callout); color: var(--text); }
	.sheet { position: relative; border-radius: 20px; background: linear-gradient(180deg, var(--sheet-a), var(--sheet-b) 420px); box-shadow: var(--edge), var(--drop); overflow: clip; }
	.hd { min-height: 40px; align-items: center; background: color-mix(in srgb, var(--text) 3%, transparent); border-bottom: 1px solid var(--line); color: color-mix(in srgb, var(--text) 78%, transparent); font: 600 12px/1 var(--font-ui); }
	.item { position: relative; transition: background-color var(--dur-fast) var(--ease-out); }
	.item + .item::before { content: ''; position: absolute; top: 0; inset-inline: 14px; height: 1px; background: var(--line); }
	.unr { background: var(--unbg); }

	.tk { flex: none; width: 18px; height: 18px; margin: 0; accent-color: var(--accent); }
	input.tk { cursor: pointer; }
	input.tk:disabled { cursor: default; opacity: 0.45; }
	input.tk:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 4px; }
	.pill { display: inline-flex; align-items: center; justify-self: start; max-width: 100%; height: 22px; padding: 0 8px; border-radius: 999px; overflow: hidden; text-overflow: ellipsis; font: 600 12px/1 var(--font-ui); white-space: nowrap; font-variant-numeric: tabular-nums; }
	.pill.ok { background: color-mix(in srgb, var(--ok) 12%, transparent); color: var(--ok-text); }
	.pill.un { background: color-mix(in srgb, var(--wait) 14%, transparent); color: var(--wait-text); }
	.pill.off { background: color-mix(in srgb, var(--text) 7%, transparent); color: var(--text-muted); }
	.pk { display: block; width: 100%; min-width: 0; max-width: 100%; height: 36px; box-sizing: border-box; padding-block: 0; padding-inline: 11px 30px; border: 0; border-radius: var(--r-btn); background-color: var(--surface-2); background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='%23888' stroke-width='2' stroke-linecap='round'%3E%3Cpath d='M6 9.5l6 6 6-6'/%3E%3C/svg%3E"); background-repeat: no-repeat; background-position: right 9px center; background-size: 14px; color: var(--text); font: var(--t-subhead); font-size: 13px; text-overflow: ellipsis; white-space: nowrap; appearance: none; -webkit-appearance: none; cursor: pointer; }
	:global([dir='rtl']) .pk { background-position: left 9px center; }
	.pk:hover { background-color: color-mix(in srgb, var(--text) 9%, var(--surface-2)); }
	.pk:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.pk.un { background-color: var(--surface-1); box-shadow: inset 0 0 0 1.5px color-mix(in srgb, var(--wait) 70%, transparent); }
	.pk.un:focus-visible { box-shadow: inset 0 0 0 1.5px color-mix(in srgb, var(--wait) 70%, transparent), var(--focus-ring); }
	.pk.still { display: flex; align-items: center; overflow: hidden; cursor: default; }
	.open { justify-self: start; }
	.why { color: var(--text-muted); font: var(--t-footnote); }
	.pos { display: block; flex: none; padding: 0; border: 0; background: var(--surface-2); aspect-ratio: var(--poster-ratio); overflow: hidden; }
	.pos :global(.poster) { width: 100%; height: 100%; border: 0; border-radius: inherit; }
	.zoom { cursor: zoom-in; }
	.zoom:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.tl { display: flex; align-items: baseline; gap: 8px; min-width: 0; }
	.tl b { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: 600 14px/1.25 var(--font-ui); color: var(--text); }
	.tl small { flex: none; color: var(--text-muted); font: 500 12.5px/1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.tl.big b { font: var(--t-title3); }
	.fd { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); font: var(--t-footnote); }
	.tx { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
	.thumb { width: 32px; border-radius: 5px; box-shadow: 0 0 0 1px var(--line); }

	/* Rows: the Library's ledger, 54px a folder. */
	.v-rows .row { display: grid; grid-template-columns: 28px 36px minmax(140px, 1.1fr) minmax(120px, 1fr) 128px minmax(200px, 260px); gap: 0 12px; align-items: center; padding-inline: 14px 12px; }
	.v-rows .item { min-height: 54px; padding-block: 8px; box-sizing: border-box; }
	.v-rows .c-sub { display: none; }
	.v-rows .more { grid-column: 3 / -1; }
	@container (max-width: 820px) {
		.v-rows .row { grid-template-columns: 28px 36px minmax(0, 1fr) auto minmax(160px, 220px); }
		.v-rows .c-fd { display: none; }
		.v-rows .c-sub { display: contents; }
	}

	/* Banners: the title's wide art set into the sheet, as in the Library. */
	.v-banners .row { display: grid; grid-template-columns: 28px var(--bw) minmax(160px, 1fr) 128px minmax(200px, 250px); gap: 0 16px; align-items: center; padding-inline: 14px 16px; }
	.v-banners .item { min-height: calc(var(--bw) / var(--bratio) + 16px); padding-block: 8px; box-sizing: border-box; }
	.v-banners .item + .item::before { inset-inline-start: calc(var(--bw) + 58px); inset-inline-end: 16px; }
	.c-st2 { display: none; }
	.ban { display: block; min-width: 0; --banner-r: 12px; border-radius: 12px; box-shadow: 0 1px 2px rgb(20 24 40 / 12%), 0 10px 18px -12px rgb(20 24 40 / 50%); aspect-ratio: var(--bratio); background: var(--surface-2); overflow: hidden; }
	@container (max-width: 980px) {
		.v-banners .row { grid-template-columns: 28px min(var(--bw), 34cqw) minmax(0, 1fr) minmax(160px, 220px); }
		.v-banners .hd > :nth-child(4), .v-banners .c-st { display: none; }
		.v-banners .c-st2 { display: block; }
		.v-banners .item + .item::before { inset-inline-start: calc(min(var(--bw), 34cqw) + 58px); }
	}

	/* Posters: the Library's poster wall; tick and status on the art, the match under it. */
	.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(var(--pmin), 1fr)); gap: 22px 18px; margin: 0; padding: 0; list-style: none; }
	.grid.sz-s { gap: 18px 14px; }
	.card { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
	.pw { position: relative; }
	.wall { width: 100%; border-radius: 12px; box-shadow: 0 1px 2px rgb(20 24 40 / 12%), 0 10px 18px -12px rgb(20 24 40 / 50%); }
	.card.unr { background: none; }
	.card.unr .wall { box-shadow: 0 0 0 2px var(--surface-1), 0 0 0 4px color-mix(in srgb, var(--wait) 75%, transparent), 0 10px 18px -12px rgb(20 24 40 / 50%); }
	.ovl { position: absolute; top: 8px; inset-inline-start: 8px; display: grid; place-items: center; width: 30px; height: 30px; border-radius: 9px; background: color-mix(in srgb, var(--surface-1) 88%, transparent); box-shadow: 0 1px 3px rgb(0 0 0 / 25%); backdrop-filter: blur(6px); }
	.ovl > span.tk { display: none; }
	.ovl:has(> span.tk) { display: none; }
	.ovr { position: absolute; bottom: 8px; inset-inline: 8px; display: flex; pointer-events: none; }
	.ovr .pill { box-shadow: 0 1px 3px rgb(0 0 0 / 25%); background: color-mix(in srgb, var(--surface-1) 92%, transparent); }
	.card .tl { margin-top: 4px; }
	.sz-s .tl b { font-size: 13px; }
	.card .pk { height: 32px; margin-top: 2px; font-size: 12.5px; }

	/* Overview: a bigger poster, the facts, and the match at the side. */
	.v-overview .row { display: grid; grid-template-columns: 28px 104px minmax(0, 1fr) minmax(220px, 280px); gap: 0 18px; align-items: start; padding-block: 16px; padding-inline: 14px 18px; }
	.v-overview .row > .tk { margin-top: 6px; }
	.op { width: 104px; border-radius: 10px; box-shadow: 0 1px 2px rgb(20 24 40 / 12%), 0 10px 18px -12px rgb(20 24 40 / 50%); }
	.tline { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 10px; min-width: 0; }
	dl { display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: 4px 14px; margin: 8px 0 0; font: var(--t-footnote); }
	dt { color: var(--text-muted); }
	dd { margin: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text); }
	.v-overview .pkw { display: flex; flex-direction: column; gap: 6px; padding-top: 4px; min-width: 0; }
	.lab { color: var(--text-muted); font: var(--t-footnote); }
	@container (max-width: 760px) {
		.v-overview .row { grid-template-columns: 28px 84px minmax(0, 1fr); }
		.v-overview .pkw { grid-column: 3; padding-top: 12px; }
		.v-overview .op { width: 84px; }
	}

	/* Phone: each view stacks, and the picker gets the full width under the folder. */
	.phone.v-rows .item { grid-template-columns: 22px 36px minmax(0, 1fr) auto; grid-template-areas: 't p x s' '. . k k' '. . m m'; gap: 6px 10px; padding: 9px 12px; }
	.phone.v-rows .item > .tk { grid-area: t; }
	.phone.v-rows .item > .thumb { grid-area: p; }
	.phone.v-rows .item > .tx { grid-area: x; }
	.phone.v-rows .item > .c-st { grid-area: s; align-self: start; }
	.phone.v-rows .item > .pkw { grid-area: k; }
	.phone.v-rows .item > .more { grid-area: m; }
	.phone.v-rows .c-fd { display: none; }
	.phone.v-rows .c-sub { display: contents; }
	.phone .pk { height: 40px; }
	.phone .item + .item::before { inset-inline: 12px; }
	.phone.v-banners .item { display: grid; grid-template-columns: 22px minmax(0, 1fr) auto; grid-template-areas: 'b b b' 't x s' '. k k'; gap: 8px 10px; padding: 0 12px 12px; min-height: 0; }
	.phone.v-banners .item > .tk { grid-area: t; align-self: center; }
	.phone.v-banners .item > .ban { grid-area: b; margin-inline: -12px; border-radius: 0; box-shadow: none; --banner-r: 0; }
	.phone.v-banners .item > .tx { grid-area: x; }
	.phone.v-banners .item > .c-st { display: block; grid-area: s; align-self: center; }
	.phone.v-banners .item > .pkw { grid-area: k; }
	.phone.v-banners .item + .item::before { display: none; }
	.phone.v-banners .item + .item { border-top: 1px solid var(--line); }
	.phone.v-banners .tl.big b { font: var(--t-headline); }
	.phone.v-banners .c-st2 { display: none; }
	.phone.v-posters .grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 18px 12px; }
	.phone.v-overview .item { grid-template-columns: 22px 76px minmax(0, 1fr); grid-template-areas: 't p x' 'k k k'; gap: 12px; padding: 14px 12px; }
	.phone.v-overview .item > .tk { grid-area: t; }
	.phone.v-overview .item > .op { grid-area: p; width: 76px; }
	.phone.v-overview .item > .tx { grid-area: x; }
	.phone.v-overview .item > .pkw { grid-area: k; flex-direction: row; align-items: center; padding: 0; padding-inline-start: 34px; }
	.phone.v-overview .pkw .lab { flex: none; }
	.phone.v-overview .tl.big b { font: var(--t-headline); white-space: normal; }
	@media (prefers-reduced-motion: reduce) { .item { transition: none; } }
</style>
