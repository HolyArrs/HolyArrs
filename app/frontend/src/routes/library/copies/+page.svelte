<script lang="ts">
	import PageLoader from '#lib/ui/PageLoader.svelte';
	import Poster from '#lib/ui/Poster.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Badge from '#lib/ui/Badge.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { LibraryToolPage, languageName, titleArt, codecLabel, copiesQuery, copyCodecs, copyKey, copySorts, toggleCopy, type CopyCodec, type LibraryCopy, type CopySearchPreview, type CopySearchResult, type CopySelection, type CopySort, type LibraryCopyPage } from '#lib/mediafiles/index.ts';
	import { DlFind } from '#lib/downloads/index.ts';
	import { onMount } from 'svelte';
	import { api, message } from '#lib/core/api.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { to } from '#lib/core/paths.ts';
	import { plainReason } from '#lib/core/plain.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	const pageSize = 50;
	let page = $state<LibraryCopyPage>({ copies: [], total: 0, limit: pageSize, offset: 0, codecs: {} });
	let selected = $state<CopySelection[]>([]);
	let preview = $state<CopySearchPreview | null>(null);
	let result = $state<CopySearchResult | null>(null);
	let q = $state('');
	let codec = $state<CopyCodec>('all');
	let sort = $state<CopySort>('title');
	let offset = $state(0);
	let loaded = $state(false);
	let busy = $state(false);
	let error = $state('');

	const selectedKeys = $derived(new Set(selected.map(copyKey)));
	const last = $derived(Math.min(page.total, page.offset + page.copies.length));

	async function load() {
		busy = true;
		try {
			page = await api.get<LibraryCopyPage>(copiesQuery({ q, codec, sort, limit: pageSize, offset }));
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			busy = false;
			loaded = true;
		}
	}

	function applyFilters() {
		offset = 0;
		void load();
	}
	// The search field filters as you type, after a short pause so each letter is not its own request.
	let typing: ReturnType<typeof setTimeout> | undefined;
	function setQ(v: string) {
		q = v.slice(0, 256);
		clearTimeout(typing);
		typing = setTimeout(applyFilters, 250);
	}
	const sortWord = (v: string) => t(`library.copies.sort.${v}`);
	// The second lines say only what is worth a word: the container by its usual name, and a read that is not complete.
	const containerWords: Record<string, string> = { matroska: 'MKV', 'matroska,webm': 'MKV', mp4: 'MP4', 'mov,mp4,m4a,3gp,3g2,mj2': 'MP4', avi: 'AVI', mpegts: 'TS' };
	const codecLine = (c: LibraryCopy) => [containerWords[c.container] ?? (c.container ? c.container.toUpperCase() : ''), c.inspection_status && c.inspection_status !== 'complete' ? c.inspection_status : ''].filter(Boolean).join(' · ') || t('library.copies.notRead');

	function toggle(copy: CopySelection) {
		selected = toggleCopy(selected, copy);
		preview = null;
		result = null;
	}

	async function review() {
		busy = true;
		try {
			preview = await api.post<CopySearchPreview>('/library/copies/search/preview', { selections: selected });
			result = null;
			error = '';
		} catch (e) {
			error = message(e);
			preview = null;
		} finally {
			busy = false;
		}
	}

	async function search() {
		if (!preview) return;
		busy = true;
		try {
			result = await api.post<CopySearchResult>('/library/copies/search', { selections: selected, review_token: preview.review_token });
			preview = null;
			error = '';
		} catch (e) {
			error = message(e);
			preview = null;
		} finally {
			busy = false;
		}
	}

	function move(by: number) {
		offset = Math.max(0, offset + by);
		void load();
	}

	// Posters for the rows, from one brief read of the Library.
	let art = $state<Map<number, string>>(new Map());
	onMount(() => { load(); void titleArt().then((m) => (art = m)); return () => clearTimeout(typing); });
</script>

<svelte:head><title>{t('library.copies.title')} · HolyArrs</title></svelte:head>

<LibraryToolPage title={t('library.copies.title')} count={loaded && !error ? t('mediafiles.unit.file.' + (page.total === 1 ? 'one' : 'other'), { n: page.total }) : ''}>
	{#snippet bar()}
		{#if session.can('manage_library')}
			<DlFind bind:term={() => q, setQ} shown={page.copies.length} total={page.total} searchKey="" placeholder={t('library.copies.search')} testid="copies-find" />
			<span class="tp-cap">
				<select aria-label={t('mediafiles.col.codec')} bind:value={codec} onchange={applyFilters}>
					{#each copyCodecs as value}<option {value}>{value === 'all' ? t('library.copies.allCodecs') : `${codecLabel(value)} (${page.codecs[value] ?? 0})`}</option>{/each}
				</select>
			</span>
			<span class="tp-cap">
				<span class="tp-lab">{t('library.copies.sort.label')}</span>
				<select aria-label={t('library.copies.sort.label')} bind:value={sort} onchange={applyFilters}>
					{#each copySorts as value}<option {value}>{sortWord(value)}</option>{/each}
				</select>
			</span>
			<span class="tp-gap"></span>
			<span class="tp-cap">
				<span class="tp-val" aria-live="polite">{t('mediafiles.select.count', { n: selected.length })}</span>
				<Button variant="primary" disabled={selected.length === 0 || busy} onclick={review}>{t('library.copies.review')}</Button>
			</span>
		{/if}
	{/snippet}

{#if !session.can('manage_library')}
	<EmptyState icon="shield" title={t('library.copies.noPermission')} />
{:else}
	{#if error}<p class="tp-msg bad" role="alert"><Icon name="alert" size={17} /><span>{plainReason(error).text}</span></p>{/if}
	{#if result}<p class="tp-msg good" role="status"><Icon name="check" size={17} />{t(result.job_ids.length === 1 ? 'library.copies.startedOne' : 'library.copies.startedMany', { n: result.job_ids.length })}</p>{/if}

	{#if preview}
		<section class="tp-sheet tp-pad review" aria-labelledby="review-title">
			<h2 id="review-title">{t(preview.copies.length === 1 ? 'library.copies.searchAgainOne' : 'library.copies.searchAgainMany', { n: preview.copies.length })}</h2>
			<p class="tp-note">{t(preview.slot_count === 1 ? 'library.copies.withRulesOne' : 'library.copies.withRulesMany', { n: preview.slot_count })}</p>
			<ul>{#each preview.copies as copy (copyKey(copy))}<li><b>{copy.title}</b><span>{copy.version} · {codecLabel(copy.codec)}</span></li>{/each}</ul>
			<div class="actions"><Button variant="plain" onclick={() => preview = null}>{t('mediafiles.dialogs.cancel')}</Button><Button variant="primary" disabled={busy} onclick={search}>{t('library.copies.start')}</Button></div>
		</section>
	{/if}

	<PageLoader loading={!loaded} label={t('library.copies.loading')}>
	{#if page.copies.length === 0}
		<EmptyState compact icon="film" title={t('library.copies.nothingMatches')} />
	{:else}
		<div class="tp-sheet table-wrap">
			<table class="tp-table copies">
				<caption class="sr-only">{t('library.copies.caption')}</caption>
				<thead><tr><th scope="col"><span class="sr-only">{t('mediafiles.toolbar.select')}</span></th><th scope="col">{t('mediafiles.kindCap.movie')}</th><th scope="col">{t('mediafiles.col.codec')}</th><th scope="col">{t('mediafiles.col.audio')}</th><th scope="col">{t('mediafiles.sheet.file')}</th></tr></thead>
				<tbody>
					{#each page.copies as copy (copyKey(copy))}
						<tr class:picked={selectedKeys.has(copyKey(copy))}>
							<td class="pick"><label><input type="checkbox" aria-label={t('library.copies.selectCopy', { title: copy.title, version: copy.version })} checked={selectedKeys.has(copyKey(copy))} disabled={copy.protected} onchange={() => toggle(copy)} /></label></td>
							<td><span class="tp-who"><span class="tp-art" style="--poster-ratio: 2 / 3"><Poster src={art.get(copy.title_id)} name={copy.title} /></span><span class="tp-tx"><a href={to(`/library/${copy.title_id}`)}>{copy.title}{#if copy.year} <span class="yr">{copy.year}</span>{/if}</a><span class="tp-sub">{copy.version}{copy.edition ? ` · ${copy.edition}` : ''}</span>{#if copy.protected}<Badge label={t('mediafiles.copy.protectedBadge')} />{/if}</span></span></td>
							<td><span class="codec">{codecLabel(copy.codec)}</span><span class="tp-sub">{codecLine(copy)}</span></td>
							<td>{copy.audio_languages.length ? copy.audio_languages.map(languageName).join(', ') : '—'}<span class="tp-sub">{t('mediafiles.unit.track.' + (copy.audio_tracks === 1 ? 'one' : 'other'), { n: copy.audio_tracks })}{copy.audio_unknown ? ` · ${t('library.copies.someUnknown')}` : ''}</span></td>
							<td class="path" title={copy.relative_path}>{copy.relative_path.split('/').pop()}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
		{#if page.total > page.copies.length}
			<nav class="tp-foot" aria-label={t('library.copies.pages')}>
				<Button icon="chevronLeft" disabled={page.offset === 0 || busy} onclick={() => move(-pageSize)}>{t('library.copies.previous')}</Button>
				<span class="tp-note">{t('library.copies.range', { from: page.offset + 1, to: last, total: page.total })}</span>
				<Button iconAfter="chevronRight" disabled={last >= page.total || busy} onclick={() => move(pageSize)}>{t('library.copies.next')}</Button>
			</nav>
		{/if}
	{/if}
	</PageLoader>
{/if}
</LibraryToolPage>

<style>
	.review h2 { margin: 0 0 4px; font: var(--t-title3); }
	.review ul { margin: 12px 0; padding: 0; list-style: none; display: grid; gap: 6px; font: var(--t-callout); }
	.review li { display: flex; flex-wrap: wrap; align-items: baseline; gap: 4px 10px; }
	.review li span { color: var(--text-muted); font: var(--t-footnote); }
	.actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 8px; }
	.table-wrap { overflow-x: auto; }
	.copies input[type='checkbox'] { width: 20px; height: 20px; accent-color: var(--accent); }
	.copies td { vertical-align: middle; }
	.copies tr.picked { background: color-mix(in srgb, var(--accent) 8%, transparent); }
	.pick { width: 1%; }
	/* On touch phones the label around the box is the 44px tap target; the drawn box and the row stay the same. */
	.pick label { display: inline-flex; }
	/* The title link keeps its place in the line; on touch phones its box is 44px tall (inline padding moves nothing). */
	@media (max-width: 767px) and (pointer: coarse) { .tp-tx > a { display: inline-block; padding-block: 13px; } }
	@media (max-width: 767px) and (pointer: coarse) { .pick label { align-items: center; justify-content: center; min-width: 44px; min-height: 44px; margin: -12px; } }
	.yr { margin-left: 4px; color: var(--text-muted); font-weight: 400; }
	.codec { font-weight: 650; }
	.path { max-width: 24rem; overflow-wrap: anywhere; color: var(--text-muted); font: var(--t-footnote); }
	@media (max-width: 700px) {
		.copies th:nth-child(4), .copies td:nth-child(4), .copies th:nth-child(5), .copies td:nth-child(5) { display: none; }
	}
</style>
