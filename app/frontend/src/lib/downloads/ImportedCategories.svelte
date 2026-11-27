<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { api } from '#lib/core/api.ts';
	import Field from '#lib/ui/Field.svelte';
	import type { DownloaderForm } from './downloaders.ts';
	let { id, form = $bindable() }: { id: number; form: DownloaderForm } = $props();
	let afterCategories = $state<string[]>([]);
	let afterCategoriesListed = $state(false);
	const afterCategoryClient = $derived(form.after_import_mode !== 'off' ? id : undefined);
	$effect(() => {
		const id = afterCategoryClient;
		let active = true;
		afterCategories = [];
		afterCategoriesListed = false;
		if (id) {
			api.get<{ listed: boolean; categories: string[] }>(`/downloaders/${id}/categories`)
				.then((r) => {
					if (!active) return;
					afterCategories = r.categories ?? [];
					afterCategoriesListed = r.listed;
				})
				.catch(() => { if (active) afterCategoriesListed = false; });
		}
		return () => { active = false; };
	});
	function afterCategoryOptions(current: string): string[] {
		const main = [form.category, form.category_movies, form.category_shows, form.category_music].map((c) => c.trim().toLowerCase());
		const choices = afterCategories.filter((c) => !main.includes(c.toLowerCase()));
		return current && !choices.includes(current) ? [current, ...choices] : choices;
	}
</script>

<Field id="dl-cat-after-mode" label={t('settings.downloaders.categoryAfterImport')}>
	<select id="dl-cat-after-mode" class="input" bind:value={form.after_import_mode} data-testid="dl-cat-after-mode">
		<option value="off">{t('settings.downloaders.afterImportOff')}</option>
		<option value="single">{t('settings.downloaders.afterImportSingle')}</option>
		<option value="type">{t('settings.downloaders.afterImportType')}</option>
	</select>
</Field>
{#if form.after_import_mode === 'single'}
	<Field id="dl-cat-after" label={t('settings.downloaders.categoryAfterImport')} hint={t('settings.downloaders.categoryAfterImportHint')}>
		{#if afterCategoriesListed}
			<select id="dl-cat-after" class="input" bind:value={form.category_after_import} aria-describedby="dl-cat-after-hint" data-testid="dl-cat-after">
				<option value="">{t('settings.downloaders.afterImportOff')}</option>
				{#each afterCategoryOptions(form.category_after_import) as category (category)}<option value={category}>{category}</option>{/each}
			</select>
		{:else}
			<input id="dl-cat-after" class="input" bind:value={form.category_after_import} maxlength="64" aria-describedby="dl-cat-after-hint" data-testid="dl-cat-after" />
		{/if}
	</Field>
{:else if form.after_import_mode === 'type'}
	<p class="hint">{t('settings.downloaders.afterImportTypeHint')}</p>
	{#each ['movies', 'shows', 'music'] as kind (kind)}
		<Field id="dl-cat-after-{kind}" label={t(kind === 'movies' ? 'settings.downloaders.categoryMovies' : kind === 'shows' ? 'settings.downloaders.categoryShows' : 'settings.downloaders.categoryMusic')}>
			{#if afterCategoriesListed}
				<select id="dl-cat-after-{kind}" class="input" bind:value={form[`category_after_import_${kind as 'movies' | 'shows' | 'music'}`]} data-testid="dl-cat-after-{kind}">
					<option value="">{t('settings.downloaders.afterImportOff')}</option>
					{#each afterCategoryOptions(form[`category_after_import_${kind as 'movies' | 'shows' | 'music'}`]) as category (category)}<option value={category}>{category}</option>{/each}
				</select>
			{:else}
				<input id="dl-cat-after-{kind}" class="input" bind:value={form[`category_after_import_${kind as 'movies' | 'shows' | 'music'}`]} maxlength="64" data-testid="dl-cat-after-{kind}" />
			{/if}
		</Field>
	{/each}
{/if}
