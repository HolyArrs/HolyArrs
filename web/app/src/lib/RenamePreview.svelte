<script lang="ts">
	// The rename preview for one title (titleId), one album of an artist
	// (albumId too) or the whole Library: the old and new names, files that
	// are skipped and why, and Apply. The preview and the rename use the same
	// planner, so this is what happens.
	import { onMount } from 'svelte';
	import { api, message } from './api.ts';
	import { live } from './live.svelte.ts';
	import { toasts } from './toasts.svelte.ts';
	import { albumSteps, fileName, followJob, renamePair, splitSteps, type JobView, type RenameStep } from './organise.ts';
	import Button from './ui/Button.svelte';
	import ProgressBar from './ui/ProgressBar.svelte';
	import Skeleton from './ui/Skeleton.svelte';

	// season narrows a show's preview to that season's files (read from the SxxEyy in their names).
	let { titleId, albumId, season, onDone }: { titleId?: number; albumId?: number; season?: number; onDone?: () => void } = $props();

	let steps = $state<RenameStep[]>([]);
	// The hash of what this preview shows: the rename does only that, and
	// fails (preview again) when the plan changed since.
	let planHash = $state('');
	let loaded = $state(false);
	let error = $state('');
	let job = $state<JobView | null>(null);
	let applying = $state(false);
	const parts = $derived(splitSteps(steps));
	const many = $derived(titleId === undefined);

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

	async function apply() {
		applying = true;
		error = '';
		job = null;
		try {
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
				toasts.push(n === 1 ? '1 file renamed.' : `${n} files renamed.`, 'ok');
				onDone?.();
			} else if (end.Error?.code === 'plan_changed') {
				error = 'Something changed since this preview, so nothing was renamed. The preview below is fresh: check it and rename again.';
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

<div class="rename" data-testid="rename-preview">
	{#if !loaded}
		<Skeleton lines={4} />
	{:else}
		<p class="small muted">
			Only files HolyArrs imported or adopted are renamed, inside the folder they are in now. Files it found on its own are never touched.
		</p>
		{#if parts.change.length === 0}
			<p class="notice" data-testid="rename-none">
				Nothing to rename: every file already has the right name.
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
	.go {
		justify-content: flex-end;
		margin-top: var(--s-4);
	}
</style>
