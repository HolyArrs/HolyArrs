<!-- Hides with Show again: the People card, a person's Change dialog and the Hide sheet share it. -->
<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import { api, message } from '#lib/core/api.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { hideLine, hideName, showAgain, type Hide } from './hidden.ts';

	let { hides, people, onshown }: { hides: Hide[]; people: ReadonlyMap<number, string>; onshown: (id: number) => void } = $props();
	let busy = $state<number | null>(null);

	async function show(h: Hide) {
		busy = h.id;
		try {
			await showAgain(api, h.id);
			onshown(h.id);
			toasts.push(t('discover.hidden.shownAgain', { name: hideName(h) }), 'ok');
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = null;
		}
	}
</script>

<ul class="rows" data-testid="hidden-list">
	{#each hides as h (h.id)}
		<li>
			<div class="grow">
				<div class="name" dir="auto">{hideName(h)}</div>
				<div class="xs muted">{hideLine(h, people)}</div>
			</div>
			<Button variant="plain" size="sm" type="button" busy={busy === h.id} disabled={busy !== null} onclick={() => show(h)} aria-label={t('discover.hidden.showAgainName', { name: hideName(h) })}>{t('discover.hidden.showAgain')}</Button>
		</li>
	{/each}
</ul>

<style>
	.name {
		font-weight: var(--weight-semibold);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	/* dir="auto" cuts a Latin name at its end in Arabic; it still lines up with the line under it. */
	:global([dir='rtl']) .name {
		text-align: right;
	}
</style>
