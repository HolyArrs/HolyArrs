<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import TagInput from '#lib/ui/TagInput.svelte';
	import { forgetTags } from '#lib/ui/tags.ts';
	import { api, message } from '#lib/core/api.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import type { TitleView } from '#lib/core/types.ts';

	// The tag box in the Tags row of a title's Library tab. Tags route the title's searches and grabs:
	// tagged Sources and Downloaders serve only titles that share a tag. Each change shows at once
	// (onchange) and is saved in turn with the newest list, so the last change made is the one kept;
	// when the newest save fails, the row goes back to what it was before that change.
	let { title, onchange }: { title: TitleView; onchange: (title: TitleView) => void } = $props();

	let seq = 0;
	let queue: Promise<void> = Promise.resolve();

	function save(next: string[]) {
		const id = title.id;
		const mine = ++seq;
		const before = title.tags ?? [];
		onchange({ ...title, tags: next });
		queue = queue.then(async () => {
			if (mine !== seq) return;
			try {
				const r = await api.put<{ title: TitleView }>(`/titles/${id}`, { tags: next });
				if (mine === seq) onchange(r.title);
				forgetTags();
			} catch (e) {
				if (mine === seq) onchange({ ...title, tags: before });
				toasts.push(t('discover.titleLibrary.couldntSave'), 'fail', { body: message(e) });
			}
		});
	}
</script>

<div class="tags"><TagInput id="title-tags" bind:value={() => title.tags ?? [], save} testid="title-tags" /></div>

<style>
	.tags { flex: 0 1 280px; min-width: 0; }
</style>
