<script lang="ts">
	// Delete a show, a season or one episode (5.2.3), or an artist, an album
	// or some tracks (4.15.4). The plan below is what the delete does: it is
	// asked again whenever a choice changes, and the delete carries out that
	// same plan.
	import { api, ApiError, message } from './api.ts';
	import { toasts } from './toasts.svelte.ts';
	import { deleteBody, deleteSummary, wholeScope, type DeleteScope, type RemovePlan } from './organise.ts';

	let {
		titleId,
		name,
		kind = 'show',
		ondone
	}: { titleId: number; name: string; kind?: 'show' | 'artist'; ondone: (plan: RemovePlan) => void } = $props();

	let dialog: HTMLDialogElement | undefined = $state();
	let scope = $state<DeleteScope>({});
	let files = $state(false);
	let never = $state(false);
	let plan = $state<RemovePlan | null>(null);
	let error = $state('');
	let busy = $state(false);
	let ask = 0;

	const whole = $derived(wholeScope(scope));
	const what = $derived(whole ? name : kind === 'artist' ? `${name} – ${scope.label ?? ''}` : `${name} ${scope.label ?? ''}`.trim());
	const parts = $derived(kind === 'artist' ? 'a track you keep' : 'an episode you keep');

	function body(apply: boolean): Record<string, unknown> {
		return deleteBody(scope, files, never, apply);
	}

	async function planIt() {
		const mine = ++ask;
		error = '';
		try {
			const p = await api.post<RemovePlan>(`/titles/${titleId}/delete`, body(false));
			if (mine === ask) plan = p;
		} catch (e) {
			if (mine === ask) error = message(e);
		}
	}

	/** open shows the dialog for a scope (nothing: the whole title). */
	export function open(s: DeleteScope = {}) {
		scope = s;
		files = false;
		never = false;
		plan = null;
		dialog?.showModal();
		void planIt();
	}

	async function go() {
		busy = true;
		error = '';
		try {
			const p = await api.post<RemovePlan>(`/titles/${titleId}/delete`, body(true));
			dialog?.close();
			const held = p.held.length
				? ` ${p.held.length === 1 ? 'Its file is' : 'Their files are'} in the holding folder${p.holding_days ? ` for ${p.holding_days} days` : ''}.`
				: ' Files stay on disk.';
			toasts.push(`${what} is deleted.${held}`, 'ok');
			ondone(p);
		} catch (e) {
			error = message(e);
			if (e instanceof ApiError && e.body.plan) plan = e.body.plan as RemovePlan;
		} finally {
			busy = false;
		}
	}
</script>

<dialog bind:this={dialog} aria-labelledby="show-delete-title" class="modal wide" data-testid="show-delete">
	<h2 id="show-delete-title" class="h2">Delete {what}?</h2>
	<p class="small muted">Read what will happen below before you press Delete. Nothing has changed yet.</p>
	<label class="checkline">
		<input type="checkbox" bind:checked={files} onchange={planIt} />
		<span
			><b>Also delete {whole ? 'its' : 'their'} files</b><br /><span class="hint"
				>Files HolyArrs imported or adopted move to the holding folder in their Library folder, and are removed for good after the holding days (7 unless
				you changed it in Settings). Files it only found are never touched, and a file holding {parts} stays. Off: every file stays where it is.</span
			></span
		>
	</label>
	{#if whole}
		<label class="checkline">
			<input type="checkbox" bind:checked={never} onchange={planIt} />
			<span><b>Add it to the Never-add list</b><br /><span class="hint">Outside lists can then never add it again. You can still add it yourself.</span></span>
		</label>
	{/if}
	<section class="plan" aria-labelledby="show-plan-title" aria-live="polite" data-testid="delete-plan">
		<h3 id="show-plan-title" class="h3">What will happen</h3>
		{#if !plan}
			{#if !error}<div class="sk" style="height: 60px"></div>{/if}
		{:else}
			<ul class="warns">
				{#each deleteSummary(plan, kind) as line (line)}<li>{line}</li>{/each}
			</ul>
			{#if plan.held.length}
				<p class="xs muted">To the holding folder:</p>
				<ul class="files">
					{#each plan.held as f (f.file_id)}<li class="xs"><span class="mono path">{f.path}</span>{#if f.episodes?.length}<span class="muted"> ({f.episodes.join(', ')})</span>{/if}</li>{/each}
				</ul>
			{/if}
			{#if plan.kept.length}
				<p class="xs muted">Staying where they are:</p>
				<ul class="files">
					{#each plan.kept as f (f.file_id)}<li class="xs"><span class="mono path">{f.path}</span> <span class="muted">({f.why})</span></li>{/each}
				</ul>
			{/if}
		{/if}
	</section>
	{#if error}<p class="form-error" role="alert">{error}</p>{/if}
	<div class="row modal-actions">
		<button class="btn btn-quiet" type="button" onclick={() => dialog?.close()}>Cancel</button>
		<button class="btn btn-danger" type="button" onclick={go} disabled={busy || !plan} aria-busy={busy}>
			{whole ? `Delete ${kind}` : 'Delete ' + (scope.label || (kind === 'artist' ? 'these tracks' : 'these episodes'))}
		</button>
	</div>
</dialog>

<style>
	.modal {
		width: min(640px, calc(100vw - 32px));
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
	.plan {
		margin-top: var(--s-4);
		padding: var(--s-3) var(--s-4);
		background: var(--surface-2);
		border-radius: var(--r-sm);
	}
	.warns {
		padding-left: var(--s-5);
		margin: var(--s-2) 0;
	}
	.files {
		list-style: none;
		padding: 0;
		margin: var(--s-1) 0 var(--s-2);
	}
	.path {
		overflow-wrap: anywhere;
	}
	.modal-actions {
		justify-content: flex-end;
		margin-top: var(--s-6);
	}
</style>
