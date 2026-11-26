<script lang="ts">
	// On a Downloader an import brought: the old app hides its key or password, so it is pasted here once.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import { hiddenWhere, saveHiddenKey } from './brought.ts';

	let { id, kind, app, needs, onsaved }: { id: number; kind: string; app: string; needs: string; onsaved: (r: { ok: boolean; text: string }) => void } = $props();
	let value = $state('');
	let busy = $state(false);
	let error = $state('');
	const isKey = $derived(needs === 'key');

	async function save() {
		if (busy || !value.trim()) return;
		busy = true;
		error = '';
		const r = await saveHiddenKey(id, value);
		busy = false;
		if (r.ok) value = '';
		else error = r.text;
		onsaved(r);
	}
</script>

<div class="hidden-key" data-testid="brought-key">
	<p class="line">{t(isKey ? 'downloads.brought.hiddenKey' : 'downloads.brought.hiddenPassword', { app, where: hiddenWhere(kind) })}</p>
	<div class="row">
		<input
			type="password"
			autocomplete="off"
			spellcheck="false"
			aria-label={t(isKey ? 'downloads.brought.keyLabel' : 'downloads.brought.passwordLabel')}
			placeholder={t(isKey ? 'downloads.brought.keyLabel' : 'downloads.brought.passwordLabel')}
			bind:value
			onkeydown={(e) => {
				if (e.key === 'Enter') {
					e.preventDefault();
					void save();
				}
			}}
		/>
		<Button size="sm" {busy} disabled={busy || !value.trim()} onclick={save}>{t('downloads.brought.save')}</Button>
	</div>
	{#if error}<p class="form-error" role="alert">{error}</p>{/if}
</div>

<style>
	.hidden-key { margin-top: 8px; display: grid; gap: 8px; min-width: 0; }
	.line { margin: 0; color: var(--text-muted); font: var(--t-footnote); overflow-wrap: anywhere; }
	.row { display: flex; gap: 8px; align-items: center; min-width: 0; }
	input { flex: 1; min-width: 0; min-height: 36px; padding: 0 12px; border-radius: 10px; border: 1px solid var(--line); background: var(--surface-1, transparent); color: var(--text); font: 400 14px/1.2 var(--font-ui); }
	.form-error { margin: 0; }
</style>
