<script lang="ts">
	// Setup's Folders step (Start fresh): a part's folder, typed or picked in the folder browser, with a live check of what HolyArrs sees there.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import FolderPopup from './FolderPopup.svelte';
	import FolderSeen from './FolderSeen.svelte';

	let { id, label, kind = '', hint = '', value = $bindable(''), error = '', disabled = false, oninput, onsave, oncancel }: { id: string; label: string; kind?: string; hint?: string; value?: string; error?: string; disabled?: boolean; oninput?: () => void; onsave?: () => void; oncancel?: () => void } = $props();

	let open = $state(false);
</script>

<div class="ff">
	<div class="box" class:bad={!!error}>
		<input {id} dir="ltr" bind:value {oninput} onkeydown={(e) => { if (e.key === 'Enter' && onsave) { e.preventDefault(); onsave(); } if (e.key === 'Escape' && oncancel) { e.preventDefault(); oncancel(); } }} aria-label={t('welcome.folder', { name: label })} placeholder={hint} autocomplete="off" spellcheck="false" aria-invalid={error ? 'true' : undefined} aria-describedby={error ? `${id}-error` : undefined} />
		<span class="tools">
			<button type="button" class="tb" {disabled} onclick={() => (open = true)} aria-label={t('welcome.folder', { name: label })} title={t('welcome.folder', { name: label })} data-testid="{id}-browse"><Icon name="folder" size={16} /></button>
			{#if onsave}<button type="button" class="tb" class:go={!!value.trim()} disabled={disabled || !value.trim()} onclick={onsave} aria-label={t('welcome.folders.save')} title={t('welcome.folders.save')} data-testid="{id}-save"><Icon name="check" size={16} /></button>{/if}
			{#if oncancel}<button type="button" class="tb" {disabled} onclick={oncancel} aria-label={t('welcome.folders.keepOld')} title={t('welcome.folders.keepOld')} data-testid="{id}-cancel"><Icon name="x" size={16} /></button>{/if}
		</span>
	</div>
	{#if error}<p class="form-error" id="{id}-error" role="alert">{error}</p>{:else}<FolderSeen path={value} {kind} {id} />{/if}
</div>
{#if open}<FolderPopup title={label} current={value.trim().startsWith('/') ? value.trim() : ''} onclose={() => (open = false)} onpick={(to) => { value = to; open = false; oninput?.(); }} />{/if}

<style>
	.ff { display: flex; flex-direction: column; gap: 6px; }
	/* One box like the saved folder rows: 44 px, same corners and line; the buttons sit inside it at the end. */
	.box { display: flex; align-items: center; gap: 4px; height: 44px; padding: 0 7px 0 12px; border-radius: 12px; border: 1px solid var(--g-line, var(--line)); background: var(--g-bg, var(--surface-1)); transition: border-color var(--dur-fast) var(--ease-out); }
	.box:focus-within { border-color: var(--g-line-hover, var(--line-strong)); box-shadow: var(--focus-ring); }
	.box.bad { border-color: color-mix(in srgb, var(--bad-text, #ff6b6b) 60%, transparent); }
	.box input { flex: 1; min-width: 0; height: 100%; border: 0; outline: none; background: none; color: var(--text); font: 400 14px/1.3 var(--font-ui); padding: 0; }
	/* The box is the field: its input stays clear when clicked (no app-wide clicked-field fill). */
	.box input:focus-visible { background: none !important; border-color: transparent !important; }
	.box input::placeholder { color: var(--text-muted); }
	.tools { flex: none; display: flex; align-items: center; gap: 2px; }
	.tb { width: 30px; height: 30px; display: grid; place-items: center; padding: 0; border: 0; border-radius: 8px; background: none; color: var(--text-muted); cursor: pointer; transition: background-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out); }
	.tb:hover:not(:disabled) { color: var(--text); background: color-mix(in srgb, var(--text) 10%, transparent); }
	.tb:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.tb:disabled { opacity: 0.4; cursor: default; }
	.tb.go { color: var(--ok-text); }
</style>
