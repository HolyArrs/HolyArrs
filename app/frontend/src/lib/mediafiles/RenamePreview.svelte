<script lang="ts">
	// Preview of file renames for a title or the whole library, with an apply step.
	import Poster from '#lib/ui/Poster.svelte';
	import Button from '#lib/ui/Button.svelte';
	import ProgressBar from '#lib/ui/ProgressBar.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import { titleArt } from './title-art.ts';
	import { albumSteps, fileName, followJob, renamePair, splitSteps, type JobView, type RenameStep } from './organise.ts';
	import { onMount } from 'svelte';
	import { api, message } from '../core/api.ts';
	import { live } from '../core/live.svelte.ts';
	import { toasts } from '../core/toasts.svelte.ts';

	let { titleId, albumId, season, onDone, page = false, changeCount = $bindable(0) }: { titleId?: number; albumId?: number; season?: number; onDone?: () => void; page?: boolean; changeCount?: number } = $props();

	let art = $state<Map<number, string>>(new Map());
	let steps = $state<RenameStep[]>([]);
	let planHash = $state('');
	let loaded = $state(false);
	let error = $state('');
	let job = $state<JobView | null>(null);
	let applying = $state(false);

	const parts = $derived(splitSteps(steps));
	// No title id means the preview covers every title, so rows show the title name.
	const many = $derived(titleId === undefined);

	$effect(() => {
		if (page) void titleArt().then((m) => (art = m));
	});
	$effect(() => {
		if (page) changeCount = parts.change.length;
	});

	// Fetches the plan; the season filter reads the S01E02 part of the old name.
	async function load() {
		try {
			const p = await api.get<{ steps: RenameStep[]; plan_hash: string }>(titleId === undefined ? '/rename' : `/titles/${titleId}/rename`);
			steps = albumSteps(p.steps, albumId).filter((s) => season === undefined || Number(/S(\d{1,3})E\d+/i.exec(s.from)?.[1] ?? -1) === season);
			planHash = p.plan_hash;
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}
	onMount(load);

	// The plan hash lets the server refuse if files changed since the preview.
	async function apply() {
		applying = true;
		error = '';
		job = null;
		try {
			// A single title sends only the files shown; a library rename sends the whole plan.
			const body = many ? { plan_hash: planHash } : { plan_hash: planHash, file_ids: parts.change.map((s) => s.file_id) };
			const r = await api.post<{ job_id: number }>(many ? '/rename' : `/titles/${titleId}/rename`, body);
			const end = await followJob(
				(id) => api.get<JobView>(`/jobs/${id}`),
				r.job_id,
				(j) => (job = j),
				{
					poke: (wake) =>
						live.on('job', (d) => {
							if ((d as { id?: number })?.id === r.job_id) wake();
						})
				}
			);
			if (end.State === 'done') {
				const n = Number(end.Result?.renamed ?? 0);
				toasts.push(n === 1 ? '1 file renamed' : `${n} files renamed`, 'ok');
				onDone?.();
			} else if (end.Error?.code === 'plan_changed') {
				error = 'Files changed, so nothing was renamed. Check the new preview and try again.';
			} else {
				error = end.Error?.message || 'The rename did not finish. Nothing was half-done; try again.';
			}
			await load();
		} catch (e) {
			error = message(e);
		} finally {
			applying = false;
		}
	}
</script>

{#if page}
<div class="rename" data-testid="rename-preview">
	{#if !loaded}
		<div class="tp-sheet tp-pad"><Skeleton lines={4} /></div>
	{:else}
		{#if parts.change.length}<p class="tp-note">Nothing changes until you press Rename.</p>{/if}
		{#if error}<p class="tp-msg bad" role="alert">{error}</p>{/if}
		{#if job && !['done', 'failed', 'cancelled'].includes(job.State)}
			<div class="tp-sheet tp-pad" aria-live="polite">
				<ProgressBar value={job.Progress * 100} label="Rename progress" />
				<p class="tp-note">{job.ProgressNote || (job.State === 'queued' ? 'Waiting to start.' : 'Renaming.')}</p>
			</div>
		{/if}
		<div class="tp-sheet">
			{#if parts.change.length === 0}
				<p class="tp-pad none" data-testid="rename-none">Nothing to rename.</p>
			{:else}
				<h2 class="sr-only">{parts.change.length === 1 ? '1 file gets a new name' : `${parts.change.length} files get a new name`}</h2>
				<ul class="tp-rows steps">
					{#each parts.change as s (s.file_id)}
						<li data-testid="rename-step" class:with-art={page}>
							{#if page}<span class="tp-art" style="--poster-ratio: 2 / 3"><Poster src={art.get(s.title_id)} name={s.title_name} /></span>{/if}
							<div class="body">
							{#if many}<b class="name">{s.title_name}</b>{/if}
							<div class="pair">
								<span class="lab">Now</span><span class="mono path old">{renamePair(s)[0]}</span>
								<span class="lab">New</span><span class="mono path new">{renamePair(s)[1]}</span>
							</div>
							{#each s.subtitles ?? [] as sub (sub.from)}
								<p class="tp-sub" data-testid="rename-subtitle">
									Subtitle {fileName(sub.from)}: {sub.to ? '→ ' + fileName(sub.to) : 'stays, ' + sub.skip}
								</p>
							{/each}
							</div>
						</li>
					{/each}
				</ul>
			{/if}
			{#if parts.skipped.length}
				<details class="skipped-d" open={parts.change.length === 0}>
					<summary>{parts.skipped.length} skipped</summary>
					<ul class="tp-rows steps">
						{#each parts.skipped as s (s.file_id)}
							<li data-testid="rename-skip">
								{#if many}<b class="name">{s.title_name}</b>{/if}
								<span class="mono path">{fileName(s.from)}</span>
								<span class="tp-sub">{s.skip}</span>
							</li>
						{/each}
					</ul>
				</details>
			{/if}
		</div>
		{#if parts.same}<p class="tp-note">{parts.same} already right.</p>{/if}
		{#if parts.change.length}
			<div class="tp-foot go-foot">
				<span></span>
				<Button variant="primary" onclick={apply} busy={applying}>
					{parts.change.length === 1 ? 'Rename 1 file' : `Rename ${parts.change.length} files`}
				</Button>
			</div>
		{/if}
	{/if}
</div>
{:else}
<div class="rename" data-testid="rename-preview">
	{#if !loaded}
		<Skeleton lines={4} />
	{:else}
		{#if parts.change.length === 0}
			<p class="notice" data-testid="rename-none">
				Every name matches.
			</p>
		{:else}
			<h3 class="h3">{parts.change.length === 1 ? '1 file gets a new name' : `${parts.change.length} files get a new name`}</h3>
			<ul class="steps">
				{#each parts.change as s (s.file_id)}
					<li data-testid="rename-step">
						{#if many}<b class="small">{s.title_name}</b>{/if}
						<div class="pair">
							<span class="xs muted lab">Now</span><span class="mono small path old">{renamePair(s)[0]}</span>
							<span class="xs muted lab">New</span><span class="mono small path new">{renamePair(s)[1]}</span>
						</div>
						{#each s.subtitles ?? [] as sub (sub.from)}
							<p class="xs muted" data-testid="rename-subtitle">
								Subtitle {fileName(sub.from)}: {sub.to ? 'follows as ' + fileName(sub.to) : 'stays, ' + sub.skip}
							</p>
						{/each}
					</li>
				{/each}
			</ul>
		{/if}
		{#if parts.skipped.length}
			<details class="skipped" open={parts.change.length === 0}>
				<summary class="small">{parts.skipped.length} skipped</summary>
				<ul class="steps">
					{#each parts.skipped as s (s.file_id)}
						<li data-testid="rename-skip">
							{#if many}<b class="small">{s.title_name}</b>{/if}
							<span class="mono small path">{fileName(s.from)}</span>
							<span class="small muted">Skipped: {s.skip}</span>
						</li>
					{/each}
				</ul>
			</details>
		{/if}
		{#if parts.same}<p class="xs muted">{parts.same} already named right.</p>{/if}
		{#if job && !['done', 'failed', 'cancelled'].includes(job.State)}
			<div aria-live="polite">
				<ProgressBar value={job.Progress * 100} label="Rename progress" />
				<p class="xs muted">{job.ProgressNote || (job.State === 'queued' ? 'Waiting to start.' : 'Renaming.')}</p>
			</div>
		{/if}
		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
		{#if parts.change.length}
			<div class="row go">
				<Button variant="primary" onclick={apply} busy={applying}>
					{parts.change.length === 1 ? 'Rename 1 file' : `Rename ${parts.change.length} files`}
				</Button>
			</div>
		{/if}
	{/if}
</div>
{/if}

<style>
	.steps {
		list-style: none;
		padding: 0;
		margin: var(--s-2) 0 0;
	}
	.steps li {
		display: flex;
		flex-direction: column;
		gap: 2px;
		padding: var(--s-3) 0;
	}
	.steps li.with-art { display: flex; align-items: flex-start; gap: 12px; }
	.steps li.with-art > .body { min-width: 0; flex: 1; }
	.steps li + li {
		border-top: 1px solid var(--line);
	}
	.pair {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr);
		column-gap: var(--s-3);
		row-gap: 2px;
		align-items: baseline;
	}
	.path {
		overflow-wrap: anywhere;
	}
	.old {
		color: var(--text-muted);
	}
	.lab {
		font-weight: 600;
	}
	.rename { display: flex; flex-direction: column; gap: 8px; min-width: 0; }
	.rename :global(.notice) { margin: 0; padding: 12px 14px; border: 0; border-radius: var(--r-btn); background: var(--surface-2); font: var(--t-callout); }
	.rename :global(.h3) { margin: 8px 0 0; font: var(--t-headline); }
	.skipped summary { min-height: 44px; display: inline-flex; align-items: center; cursor: pointer; }
	.form-error { margin: 0; color: var(--fail); font: var(--t-callout); }

	.skipped {
		margin-top: var(--s-3);
	}

	.rename :global(.tp-rows.steps) { margin: 0; }
	.rename :global(.tp-rows.steps > li) { display: flex; flex-direction: column; gap: 4px; }
	.name { font: 600 14px/1.3 var(--font-ui); overflow-wrap: anywhere; }
	.rename .pair .lab { color: var(--text-muted); font: 500 12px/1.5 var(--font-ui); }
	.rename .pair .path { font: 400 13px/1.5 var(--font-mono); }
	.rename .pair .old { color: var(--text-muted); }
	.none { margin: 0; color: var(--text-muted); font: var(--t-callout); }
	.skipped-d { border-top: 1px solid var(--line); }
	.skipped-d:first-child { border-top: 0; }
	.skipped-d summary { display: flex; align-items: center; min-height: 44px; padding: 0 20px; cursor: pointer; color: var(--text-muted); font: var(--t-callout); }
	.skipped-d[open] > summary { border-bottom: 1px solid var(--line); }
	.go-foot :global(.ui-btn) { min-height: 44px; }
	@media (max-width: 639px) {
		.skipped-d summary { padding: 0 16px; }
		.go-foot { justify-content: flex-end; }
	}
	.go {
		justify-content: flex-end;
		margin-top: var(--s-4);
	}
</style>
