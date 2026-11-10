<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import WishEditor from './LanguageWants.svelte';
	import {
		fileName,
		fromWords,
		kindLabel,
		langName,
		missing,
		missingWords,
		offsetWords,
		ours,
		passes,
		present,
		scoreLines,
		sourceLabel,
		syncBody,
		wantLabel,
		wishLine,
		wishProblem,
		type Choices,
		type Language,
		type SubtitleRow,
		type SubtitleSettings,
		type TitleSubtitles,
		type Want
	} from './api.ts';
	import { onMount } from 'svelte';
	import { to } from '../core/paths.ts';
	import { api, message } from '../core/api.ts';
	import { session } from '../core/session.svelte.ts';
	import { dateTime } from '../core/format.ts';
	import { live } from '../core/live.svelte.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import type { EventRecord } from '../core/types.ts';

	let {
		titleId,
		movie,
		itemId = 0,
		label = '',
		files: showFiles = true,
		bare = false,
		missingCount = $bindable(0)
	}: { titleId: number; movie: boolean; itemId?: number; label?: string; files?: boolean; bare?: boolean; missingCount?: number } = $props();

	const idp = $derived(itemId ? `sub-e${itemId}` : `sub-t${titleId}`);
	let data = $state<TitleSubtitles | null>(null);
	let langs = $state<Language[]>([]);
	let set = $state<Pick<SubtitleSettings, 'pass_mark_movie' | 'pass_mark_episode' | 'sync_from_fps' | 'sync_to_fps' | 'sync_step_ms' | 'languages'> | null>(null);
	let error = $state('');
	let searching = $state(false);

	let editing = $state(false);
	let own = $state(false);
	let draft = $state<Want[]>([]);
	let wishError = $state('');
	let wishBusy = $state(false);

	let choosing = $state(0);
	let choices = $state<Choices | null>(null);
	let choicesError = $state('');
	let choosingKey = $state('');
	let syncing = $state(0);
	let offset = $state(0);
	let fps = $state({ on: false, from: 25, to: 23.976 });
	let syncError = $state('');
	let turning = $state(0);
	let reason = $state('');
	let turnError = $state('');
	let busy = $state(false);

	const passMark = $derived(set ? (movie ? set.pass_mark_movie : set.pass_mark_episode) : 0);
	const step = $derived(set?.sync_step_ms ?? 500);
	const myFiles = $derived((data?.files ?? []).filter((f) => !itemId || f.item_id === itemId));
	const ownRow = $derived((data?.wishes ?? []).find((w) => (itemId ? w.item_id === itemId : !w.item_id && !w.version_id)));
	const resolved = $derived(myFiles[0]?.wish);
	const shown = $derived.by(() => {
		if (ownRow) return { languages: ownRow.languages, from: itemId ? 'episode' : 'title' };
		if (resolved) return resolved;
		return set ? { languages: set.languages, from: 'default' } : null;
	});

	async function load() {
		try {
			data = await api.get<TitleSubtitles>(`/titles/${titleId}/subtitles`);
			missingCount = myFiles.reduce((n, f) => n + missing(f.subtitles).length, 0);
			error = '';
		} catch (e) {
			error = message(e);
		}
	}

	onMount(() => {
		void load();
		api.get<{ languages: Language[] }>('/subtitles/languages').then((r) => (langs = r.languages), () => {});
		// without the settings permission the panel keeps its sync defaults
		if (session.can('settings')) api.get<SubtitleSettings>('/settings/subtitles').then(
			(s) => {
				set = s;
				fps = { on: false, from: s.sync_from_fps, to: s.sync_to_fps };
			},
			() => {}
		);
		let timer: ReturnType<typeof setTimeout> | undefined;
		const soon = () => {
			clearTimeout(timer);
			timer = setTimeout(load, 300);
		};
		const offs = [
			live.on('event', (d) => {
				const e = d as EventRecord;
				if (e?.TitleID === titleId && String(e.Kind).startsWith('subtitle_')) soon();
			}),
			live.on('resync', soon)
		];
		return () => {
			clearTimeout(timer);
			offs.forEach((off) => off());
		};
	});

	function startWish() {
		own = !!ownRow;
		draft = (ownRow?.languages ?? shown?.languages ?? []).map((w) => ({ ...w }));
		wishError = '';
		editing = true;
	}

	async function saveWish(e: SubmitEvent) {
		e.preventDefault();
		if (own) {
			wishError = wishProblem(draft);
			if (wishError) return;
		}
		wishBusy = true;
		try {
			const body: Record<string, unknown> = own ? { languages: draft } : { inherit: true };
			if (itemId) body.item_id = itemId;
			await api.put(`/titles/${titleId}/subtitle-wish`, body);
			toasts.push(own ? t('subtitles.panel.saved') : t('subtitles.panel.usingDefault'), 'ok');
			editing = false;
			await load();
		} catch (err) {
			wishError = message(err);
		} finally {
			wishBusy = false;
		}
	}

	async function search() {
		searching = true;
		try {
			const body: Record<string, number> = { title_id: titleId };
			if (itemId) body.item_id = itemId;
			const r = await api.post<{ queued: number }>('/subtitles/search', body);
			toasts.push(r.queued ? t('subtitles.panel.searching') : t('subtitles.panel.noFile'), r.queued ? 'info' : 'ok');
		} catch (e) {
			toasts.push(t('subtitles.panel.searchFailed'), 'fail', { body: message(e) });
		} finally {
			searching = false;
		}
	}

	function closeForms() {
		choosing = 0;
		syncing = 0;
		turning = 0;
	}

	async function openChoose(fileId: number) {
		closeForms();
		choosing = fileId;
		choices = null;
		choicesError = '';
		try {
			choices = await api.post<Choices>(`/files/${fileId}/subtitles/choices`);
		} catch (e) {
			choicesError = message(e);
		}
	}

	async function choose(fileId: number, site: string, key: string) {
		choosingKey = key;
		try {
			const r = await api.post<{ path: string }>(`/files/${fileId}/subtitles/choose`, { site, key });
			toasts.push(t('subtitles.panel.chosen'), 'ok', { body: fileName(r.path) });
			choosing = 0;
			await load();
		} catch (e) {
			choicesError = message(e);
		} finally {
			choosingKey = '';
		}
	}

	function openSync(s: SubtitleRow) {
		closeForms();
		syncing = s.ID;
		offset = 0;
		fps = { on: false, from: set?.sync_from_fps ?? 25, to: set?.sync_to_fps ?? 23.976 };
		syncError = '';
	}

	async function sync(e: SubmitEvent) {
		e.preventDefault();
		const body = syncBody(Number(offset), fps);
		if (!body.offset_ms && !body.from_fps) {
			syncError = t('subtitles.panel.shiftFirst');
			return;
		}
		busy = true;
		try {
			await api.post(`/subtitles/${syncing}/sync`, body);
			toasts.push(t('subtitles.panel.timingFixed'), 'ok', { body: t('subtitles.panel.oldInHolding') });
			syncing = 0;
			await load();
		} catch (err) {
			syncError = message(err);
		} finally {
			busy = false;
		}
	}

	function openTurn(s: SubtitleRow) {
		closeForms();
		turning = s.ID;
		reason = '';
		turnError = '';
	}

	async function turnDown(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		try {
			await api.post(`/subtitles/${turning}/turn-down`, { reason: reason.trim() });
			toasts.push(t('subtitles.panel.turnedDown'), 'ok', { body: t('subtitles.panel.nextRelease') });
			turning = 0;
			await load();
		} catch (err) {
			turnError = message(err);
		} finally {
			busy = false;
		}
	}
</script>

<section class="card subs" aria-labelledby="{idp}-h" data-testid="subtitles">
	<div class="card-head" class:bare>
		<h2 id="{idp}-h" class="h2" class:sr-only={bare}>{label ? t('subtitles.panel.titleOf', { label }) : t('subtitles.panel.title')}</h2>
		{#if !session.held('subtitle_sites')}<Button size="sm" icon="search" onclick={search} disabled={searching} busy={searching} data-testid="subtitle-search">
			{t('subtitles.panel.look')}
		</Button>{/if}
	</div>
	{#if error}
		<p class="notice bad" role="alert">{error}</p>
	{:else if !data}
		<div class="sk" style="height: 60px"></div>
	{:else}
		{#if !editing}
			<div class="wish">
				<p class="small" data-testid="subtitle-wish">
					<b>{t('subtitles.panel.languages')}</b>
					<span class="wish-v">{shown ? wishLine(shown.languages, langs) : '…'}{#if shown}{' · '}<span class="muted">{fromWords(shown.from, movie)}</span>{/if}</span>
				</p>
				<Button variant="plain" size="sm" onclick={startWish} aria-label={t('subtitles.panel.changeLanguages')} data-testid="subtitle-wish-change">{t('subtitles.panel.change')}</Button>
			</div>
		{:else}
			<form class="wish-form" onsubmit={saveWish} aria-label={t('subtitles.panel.languagesForm')}>
				<fieldset class="group">
					<legend class="label">{t('subtitles.panel.which')}</legend>
					<label class="checkline">
						<input type="radio" name="{idp}-own" checked={!own} onchange={() => (own = false)} />
						<span>{itemId ? t('subtitles.panel.sameAsShow') : t('subtitles.panel.defaultLanguages')}<br /><span class="hint">{itemId ? t('subtitles.panel.followsShow') : t('subtitles.panel.followsSettings')}</span></span>
					</label>
					<label class="checkline">
						<input type="radio" name="{idp}-own" checked={own} onchange={() => (own = true)} />
						<span>{t('subtitles.panel.own')}<br /><span class="hint">{itemId ? t('subtitles.panel.emptyEpisode') : movie ? t('subtitles.panel.emptyMovie') : t('subtitles.panel.emptyShow')}</span></span>
					</label>
				</fieldset>
				{#if own}<WishEditor bind:wants={draft} {langs} id="{idp}-w" />{/if}
				{#if wishError}<p class="form-error" role="alert">{wishError}</p>{/if}
				<div class="row actions">
					<Button variant="plain" onclick={() => (editing = false)}>{t('subtitles.panel.cancel')}</Button>
					<Button variant="primary" type="submit" disabled={wishBusy} busy={wishBusy}>{t('subtitles.panel.saveLanguages')}</Button>
				</div>
			</form>
		{/if}

		{#if !showFiles}
			<p class="small missing-line">
				{#if missingCount}
					<a href={to(`/library/missing-subtitles?title=${titleId}`)} data-testid="subtitle-missing-link">{t(missingCount === 1 ? 'subtitles.panel.missing.one' : 'subtitles.panel.missing.other', { count: missingCount })}</a>
					<span class="muted">{t('subtitles.panel.acrossEpisodes')}</span>
				{:else}
					<span class="muted">{t('subtitles.panel.noneMissing')}</span>
				{/if}
			</p>
		{:else}
			{#each myFiles as f (f.file_id)}
				{@const have = present(f.subtitles)}
				{@const lack = missing(f.subtitles)}
				<div class="file" data-testid="subtitle-file">
					<p class="mono xs muted path">{fileName(f.relative_path)}</p>
					{#if have.length === 0}
						<p class="small muted">{t('subtitles.panel.noneYet')}</p>
					{:else}
						<ul class="have">
							{#each have as s (s.ID)}
								<li data-testid="subtitle-row">
									<div class="sub-top">
										<span class="grow">
											<b class="small">{langName(s.language, langs)}</b>
											<span class="chip">{kindLabel(s)}</span>
											{#if s.score !== undefined && s.score !== null}<span class="chip {passes(s.score, passMark) ? 'ok' : 'wait'}" title={t('subtitles.panel.scoreTitle')}>{s.score}/100</span>{/if}
										</span>
										{#if ours(s)}
											<span class="row sub-actions">
												{#if !session.held('subtitle_sites')}<Button variant="plain" size="sm" onclick={() => openSync(s)} aria-label={t('subtitles.panel.syncFor', { name: langName(s.language, langs) })} data-testid="subtitle-sync">{t('subtitles.panel.syncFix')}</Button>{/if}
												{#if !session.held('subtitle_sites')}<Button variant="plain" size="sm" onclick={() => openTurn(s)} aria-label={t('subtitles.panel.turnDownFor', { name: langName(s.language, langs) })} data-testid="subtitle-turn-down">{t('subtitles.panel.turnDown')}</Button>{/if}
											</span>
										{/if}
									</div>
									<p class="xs muted">
										{sourceLabel(s)}{#if s.saved_at} · {t('subtitles.panel.savedAt', { when: dateTime(s.saved_at) })}{/if}{#if s.relative_path} · <span class="mono">{fileName(s.relative_path)}</span>{/if}
									</p>
									{#if syncing === s.ID}
										<form class="inline" onsubmit={sync} aria-label={t('subtitles.panel.syncFix')}>
											<p class="small">{t('subtitles.panel.syncIntro')}</p>
											<div class="field">
												<label for="{idp}-off-{s.ID}">{t('subtitles.panel.shift')}</label>
												<div class="row">
													<Button variant="plain" size="sm" onclick={() => (offset = Number(offset) - step)}>{t('subtitles.panel.msEarlier', { ms: step })}</Button>
													<input id="{idp}-off-{s.ID}" class="input narrow" type="number" min="-600000" max="600000" step="10" bind:value={offset} />
													<Button variant="plain" size="sm" onclick={() => (offset = Number(offset) + step)}>{t('subtitles.panel.msLater', { ms: step })}</Button>
												</div>
												<span class="hint" aria-live="polite">{t('subtitles.panel.offsetLine', { words: offsetWords(Number(offset)) })}</span>
											</div>
											<label class="checkline">
												<input type="checkbox" bind:checked={fps.on} />
												<span>{t('subtitles.panel.changeFps')}</span>
											</label>
											{#if fps.on}
												<div class="row fps">
													<div class="field">
														<label for="{idp}-from-{s.ID}">{t('subtitles.panel.fpsFrom')}</label>
														<input id="{idp}-from-{s.ID}" class="input narrow" type="number" min="1" max="120" step="0.001" bind:value={fps.from} />
													</div>
													<div class="field">
														<label for="{idp}-to-{s.ID}">{t('subtitles.panel.fpsTo')}</label>
														<input id="{idp}-to-{s.ID}" class="input narrow" type="number" min="1" max="120" step="0.001" bind:value={fps.to} />
													</div>
												</div>
											{/if}
											{#if syncError}<p class="form-error" role="alert">{syncError}</p>{/if}
											<div class="row actions">
												<Button variant="plain" onclick={() => (syncing = 0)}>{t('subtitles.panel.cancel')}</Button>
												<Button variant="primary" type="submit" disabled={busy} busy={busy}>{t('subtitles.panel.fixTiming')}</Button>
											</div>
										</form>
									{/if}
									{#if turning === s.ID}
										<form class="inline" onsubmit={turnDown} aria-label={t('subtitles.panel.turnDown')}>
											<p class="small">{t('subtitles.panel.turnIntro')}</p>
											<div class="field">
												<label for="{idp}-why-{s.ID}">{t('subtitles.panel.why')}</label>
												<input id="{idp}-why-{s.ID}" class="input" maxlength="200" bind:value={reason} placeholder={t('subtitles.panel.whyPlaceholder')} />
											</div>
											{#if turnError}<p class="form-error" role="alert">{turnError}</p>{/if}
											<div class="row actions">
												<Button variant="plain" onclick={() => (turning = 0)}>{t('subtitles.panel.cancel')}</Button>
												<Button variant="primary" type="submit" disabled={busy} busy={busy} data-testid="subtitle-turn-down-yes">{t('subtitles.panel.turnItDown')}</Button>
											</div>
										</form>
									{/if}
								</li>
							{/each}
						</ul>
					{/if}
					{#if lack.length}
						<ul class="lack">
							{#each lack as m (m.ID)}
								<li class="small" data-testid="subtitle-missing"><b>{wantLabel({ language: m.language, hearing_impaired: m.hearing_impaired ? 'hi' : 'normal', forced: m.forced }, langs)}</b>: <span class="muted">{missingWords(m, dateTime)}</span></li>
							{/each}
						</ul>
					{/if}
					<div class="row file-actions">
						{#if !session.held('subtitle_sites')}<Button size="sm" onclick={() => (choosing === f.file_id ? (choosing = 0) : openChoose(f.file_id))} aria-expanded={choosing === f.file_id} data-testid="subtitle-choose">
							{t('subtitles.panel.chooseOne')}
						</Button>{/if}
					</div>
					{#if choosing === f.file_id}
						<div class="choices" aria-live="polite" data-testid="subtitle-choices">
							{#if choicesError}
								<p class="form-error" role="alert">{choicesError}</p>
							{:else if !choices}
								<p class="small muted">{t('subtitles.panel.asking')}</p>
								<div class="sk" style="height: 60px"></div>
							{:else if choices.outcomes.length === 0}
								<p class="small muted">{t('subtitles.panel.noWished')}</p>
							{:else}
								<p class="xs muted">
									{t('subtitles.panel.asked', { sites: choices.sites_asked.length ? choices.sites_asked.join(', ') : t('subtitles.panel.noSite') })}
									{#if passMark}{t('subtitles.panel.passMark', { mark: passMark })}{/if}
								</p>
								{#each choices.outcomes as o, oi (oi)}
									<h3 class="h3 lang">{wantLabel(o.want, langs)}</h3>
									{#if !o.choices?.length}
										<p class="small muted">{o.reason || t('subtitles.panel.nothingFound')}</p>
									{:else}
										<ul class="cands" data-lang={o.want.language}>
											{#each o.choices as c (c.site + c.key)}
												<li data-testid="subtitle-choice">
													<div class="sub-top">
														<span class="grow">
															<span class="mono small rel">{c.release_names?.[0] ?? c.title ?? c.key}</span>
															<span class="xs muted">{c.site}{#if c.hearing_impaired} · {t('subtitles.panel.cHi')}{/if}{#if c.forced} · {t('subtitles.panel.cForced')}{/if}{#if c.trusted} · {t('subtitles.panel.cTrusted')}{/if}{#if c.downloads} · {t('subtitles.panel.cDownloads', { count: c.downloads })}{/if}</span>
														</span>
														<span class="chip {c.score.refused ? 'fail' : passes(c.score.total, passMark) ? 'ok' : 'wait'}" data-testid="subtitle-choice-score">{c.score.total}/100</span>
														<Button variant="plain" size="sm" disabled={!!choosingKey} busy={choosingKey === c.key} onclick={() => choose(f.file_id, c.site, c.key)} aria-label={t('subtitles.panel.chooseFrom', { name: c.release_names?.[0] ?? c.key, site: c.site })}>{t('subtitles.panel.choose')}</Button>
													</div>
													<ul class="reasons">
														{#if c.score.refused}<li class="fail">{t('subtitles.panel.scoresZero', { why: c.score.refused })}</li>{/if}
														{#each scoreLines(c.score.parts) as r, ri (ri)}<li class={r.matched ? 'ok' : ''}>{r.text}</li>{/each}
													</ul>
												</li>
											{/each}
										</ul>
									{/if}
								{/each}
							{/if}
						</div>
					{/if}
				</div>
			{:else}
				<p class="small muted">{t('subtitles.panel.noFileYet')}</p>
			{/each}
		{/if}
	{/if}
</section>

<style>
	.card-head.bare { justify-content: flex-end; }
	.subs .card-head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: var(--s-3);
		margin-bottom: 12px;
	}
	.wish p {
		display: grid;
		gap: 3px;
		margin: 0;
	}
	.wish p b {
		font: 600 15px/1.2 var(--font-ui);
	}
	.wish-v {
		color: var(--text-muted);
		font: 500 13px/1.3 var(--font-ui);
	}
	.subs .card-head .h2 {
		font: 700 20px/1.2 var(--font-ui);
		letter-spacing: -.01em;
	}
	.wish,
	.missing-line,
	.file {
		margin: 0;
		padding: 14px 20px;
		border: 0;
		border-radius: 16px;
		background: var(--surface-2);
	}
	.wish {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: var(--s-3);
	}
	.wish + .missing-line,
	.wish + .file,
	.wish-form + .missing-line,
	.wish-form + .file,
	.file + .file {
		margin-top: 8px;
	}
	.wish-form,
	.inline {
		margin-top: var(--s-3);
		padding: var(--s-4);
		border: 1px solid var(--line);
		border-radius: 6px;
		background: var(--surface-2, var(--surface-3));
	}
	.group {
		border: 0;
		padding: 0;
		margin: 0;
	}
	.actions {
		justify-content: flex-end;
		margin-top: var(--s-4);
		flex-wrap: wrap;
	}
	.path {
		overflow-wrap: anywhere;
	}
	.have,
	.cands {
		display: flex;
		flex-direction: column;
		gap: var(--s-3);
		margin-top: var(--s-2);
	}
	.sub-top {
		display: flex;
		align-items: center;
		gap: var(--s-2);
		flex-wrap: wrap;
	}
	.grow {
		flex: 1;
		min-width: 0;
		display: flex;
		align-items: center;
		gap: var(--s-2);
		flex-wrap: wrap;
	}
	.sub-actions {
		gap: var(--s-1);
		flex-wrap: wrap;
	}
	.lack {
		margin-top: var(--s-2);
	}
	.file-actions {
		margin-top: var(--s-3);
	}
	.choices {
		margin-top: var(--s-3);
	}
	.lang {
		margin-top: var(--s-4);
	}
	.rel {
		overflow-wrap: anywhere;
	}
	.narrow {
		max-width: 140px;
	}
	.fps {
		gap: var(--s-3);
		flex-wrap: wrap;
	}
	.fps .field + .field {
		margin-top: 0;
	}
	.reasons {
		margin-top: var(--s-2);
		font-size: var(--text-xs);
		color: var(--text-muted);
	}
	.reasons li {
		padding: 2px 0 2px var(--s-3);
		border-left: 2px solid var(--line-strong);
	}
	.reasons li.ok {
		border-left-color: var(--ok);
	}
	.reasons li.fail {
		border-left-color: var(--fail);
	}
	@media (max-width: 600px) {
		.subs .card-head, .wish { align-items: flex-start; flex-direction: column; }
		.sub-actions, .file-actions { width: 100%; }
		.sub-actions :global(.ui-btn), .file-actions :global(.ui-btn) { min-height: var(--tap); }
		.narrow { max-width: none; width: 100%; }
	}
</style>
