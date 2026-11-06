<script lang="ts">
	import Button from '#lib/ui/Button.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
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

	let isOpen = $state(false);
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

	function flip(which: 'files' | 'never') {
		if (which === 'files') files = !files;
		else never = !never;
		void planIt();
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
		isOpen = true;
		void planIt();
	}

	async function go() {
		busy = true;
		error = '';
		try {
			const p = await api.post<RemovePlan>(`/titles/${titleId}/delete`, body(true));
			isOpen = false;
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

<!-- The same window as the title's Delete (Luis 2026-11-06): switch rows, what will happen, then Delete. -->
<Sheet bind:open={isOpen} title={`Delete ${what}?`} size="md">
	<div class="sd" data-testid="show-delete">
		<div class="sd-rows">
			<button type="button" class="sd-row" role="switch" aria-checked={files} onclick={() => flip('files')}>
				<span><b>Also delete {whole ? 'its' : 'their'} files</b><small>Kept in the holding folder for a while</small></span>
				<span class="sd-switch" class:on={files} aria-hidden="true"></span>
			</button>
			{#if whole}
				<button type="button" class="sd-row" role="switch" aria-checked={never} onclick={() => flip('never')}>
					<span><b>Never add it again</b><small>Outside lists skip it</small></span>
					<span class="sd-switch" class:on={never} aria-hidden="true"></span>
				</button>
			{/if}
		</div>
		<section class="sd-plan" aria-live="polite" data-testid="delete-plan">
			<p class="sd-h">What will happen</p>
			{#if !plan}
				{#if !error}<div class="sd-skel"></div>{/if}
			{:else}
				<ul>{#each deleteSummary(plan, kind) as line (line)}<li>{line}</li>{/each}</ul>
				{#if plan.held.length}
					<details class="sd-files"><summary>{plan.held.length === 1 ? '1 file to the holding folder' : `${plan.held.length} files to the holding folder`}</summary>
						<ul>{#each plan.held as f (f.file_id)}<li class="mono">{f.path}</li>{/each}</ul>
					</details>
				{/if}
				{#if plan.kept.length}
					<details class="sd-files"><summary>{plan.kept.length === 1 ? '1 file stays' : `${plan.kept.length} files stay`}</summary>
						<ul>{#each plan.kept as f (f.file_id)}<li class="mono">{f.path} <span class="muted">({f.why})</span></li>{/each}</ul>
					</details>
				{/if}
			{/if}
		</section>
		{#if error}<p class="form-error" role="alert">{error}</p>{/if}
		<div class="sd-actions">
			<Button variant="plain" onclick={() => (isOpen = false)}>Cancel</Button>
			<Button variant="destructive" onclick={go} disabled={busy || !plan} busy={busy}>
				{whole ? `Delete ${kind}` : 'Delete ' + (scope.label || (kind === 'artist' ? 'these tracks' : 'these episodes'))}
			</Button>
		</div>
	</div>
</Sheet>

<style>
	.sd { display: grid; gap: 14px; }
	.sd-rows { display: grid; border-radius: var(--r-panel); background: var(--surface-2); overflow: hidden; }
	.sd-row { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 14px 20px; border: 0; background: none; color: var(--text); text-align: left; cursor: pointer; font: inherit; }
	.sd-row + .sd-row { box-shadow: inset 0 .5px 0 color-mix(in srgb, var(--text) 10%, transparent); }
	.sd-row:hover { background: color-mix(in srgb, var(--text) 4%, transparent); }
	.sd-row:focus-visible { outline: none; box-shadow: inset 0 0 0 2px var(--accent); }
	.sd-row span:first-child { display: grid; gap: 3px; }
	.sd-row b { font: 600 15px/1.2 var(--font-ui); }
	.sd-row small { color: var(--text-muted); font: 500 13px/1.3 var(--font-ui); }
	.sd-switch { position: relative; flex: none; width: 44px; height: 26px; border-radius: 99px; background: color-mix(in srgb, var(--text) 18%, transparent); transition: background var(--dur-fast) var(--ease-out); }
	.sd-switch::after { content: ''; position: absolute; top: 3px; left: 3px; width: 20px; height: 20px; border-radius: 99px; background: #fff; box-shadow: 0 1px 3px rgb(0 0 0 / 25%); transition: transform var(--dur-fast) var(--ease-out); }
	.sd-switch.on { background: var(--accent-fill); }
	.sd-switch.on::after { transform: translateX(18px); }
	.sd-plan { display: grid; gap: 8px; padding: 14px 20px; border-radius: var(--r-panel); background: var(--surface-2); }
	.sd-h { margin: 0; color: var(--text-muted); font: 600 13px/1 var(--font-ui); }
	.sd-plan ul { margin: 0; padding-left: 18px; display: grid; gap: 4px; font: 500 14px/1.4 var(--font-ui); }
	.sd-files summary { cursor: pointer; color: var(--text-muted); font: 600 13px/1.3 var(--font-ui); }
	.sd-files ul { margin-top: 6px; padding-left: 0; list-style: none; max-height: 160px; overflow: auto; }
	.sd-files .mono { font: 500 12px/1.5 var(--font-mono, ui-monospace, monospace); color: var(--text-muted); overflow-wrap: anywhere; }
	.sd-skel { height: 48px; border-radius: 10px; background: color-mix(in srgb, var(--text) 6%, transparent); }
	.sd-actions { display: flex; justify-content: flex-end; gap: 8px; }
</style>
