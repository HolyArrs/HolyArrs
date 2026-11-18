<script lang="ts">
	// Asks in the person's row before they are deleted. It opens with focus on Keep.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';

	let { name, busy, onyes, onno }: { name: string; busy: boolean; onyes: () => void; onno: () => void } = $props();
	const id = $props.id();
	const focusKeep = (el: HTMLElement) => {
		el.querySelector<HTMLElement>('[data-testid="user-delete-keep"]')?.focus();
	};
</script>

<div class="delete-ask" role="alertdialog" tabindex="-1" aria-labelledby="{id}-t" aria-describedby="{id}-d" data-testid="user-delete-ask" {@attach focusKeep}
	onkeydown={(e) => e.key === 'Escape' && onno()}>
	<p class="small" id="{id}-t">{t('settings.users.confirmDelete', { name })}</p>
	<p class="small muted" id="{id}-d">{t('settings.users.confirmDeleteKeys')}</p>
	<div class="row">
		<Button variant="destructive" type="button" onclick={onyes} disabled={busy} busy={busy} data-testid="user-delete-yes">{t('settings.users.deleteYes')}</Button>
		<Button variant="plain" type="button" onclick={onno} data-testid="user-delete-keep">{t('settings.users.keep')}</Button>
	</div>
</div>

<style>
	.delete-ask {
		flex-basis: 100%;
		display: grid;
		gap: var(--s-2);
		padding: var(--s-3);
		border: 1px solid var(--line);
		border-radius: var(--r-2, 8px);
	}
	.delete-ask p {
		margin: 0;
	}
</style>
