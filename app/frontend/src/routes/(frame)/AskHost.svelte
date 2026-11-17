<!-- Shows the question from ask() as the app's own small sheet. -->
<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { answer, asking } from '#lib/ui/ask.svelte.ts';

	let open = $state(false);
	let said = $state('');
	let yes = $state('');
	let danger = $state(false);
	$effect(() => {
		if (asking.now) {
			said = asking.now.message;
			yes = asking.now.yes || t('ui.confirm.confirm');
			danger = asking.now.danger;
			open = true;
		}
	});
	function done(yes: boolean) {
		open = false;
		answer(yes);
	}
</script>

<Sheet bind:open title={t('ui.confirm.confirm')} hideTitle size="sm" placement="center" onclose={() => asking.now && answer(false)}>
	<p class="ask-text" data-testid="ask-text">{said}</p>
	{#snippet footer()}
		<Button variant="secondary" onclick={() => done(false)} data-testid="ask-no">{t('ui.confirm.cancel')}</Button>
		<Button variant={danger ? 'destructive' : 'primary'} onclick={() => done(true)} data-testid="ask-yes">{yes}</Button>
	{/snippet}
</Sheet>

<style>
	.ask-text { font: var(--t-body); color: var(--text); white-space: pre-line; }
</style>
