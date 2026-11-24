<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { api } from '#lib/core/api.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import Field from '#lib/ui/Field.svelte';
	// The category for each kind, chosen from the download app's own list. HolyArrs fills in the one
	// that fits when the Downloader is saved; a new one shows that it will. Where the list can't be read, plain text boxes.
	// save: each change is kept at once (the setup step has no Save button).
	let {
		id,
		name = '',
		main,
		movies = $bindable(''),
		shows = $bindable(''),
		music = $bindable(''),
		save = false
	}: { id: number; name?: string; main: string; movies?: string; shows?: string; music?: string; save?: boolean } = $props();
	let listed = $state<boolean | null>(null);
	let cats = $state<string[]>([]);
	const pre = $derived(save ? 'dl-kc-' + id : 'dl-cat');

	$effect(() => {
		const want = id;
		if (!want) return;
		listed = null;
		api.get<{ listed: boolean; categories: string[]; picks?: Record<string, string> }>(`/downloaders/${want}/categories`)
			.then((r) => {
				cats = r.categories ?? [];
				listed = r.listed;
				// Nothing chosen yet (saved before picks, or the client was slow on save):
				// pre-fill the client's fitting ones. A kind someone set, even to "Same as", stays.
				const p = r.picks ?? {};
				if (r.listed && !movies && !shows && !music) {
					for (const k of kinds) {
						const v = p[k.kind];
						if (v) {
							set(k.key, v);
							void keep(k.key, k.field, v, '');
						}
					}
				}
			})
			.catch(() => (listed = false));
	});

	// Options: the app's list, plus the current one if the app no longer has it.
	function options(cur: string): string[] {
		// Exact: qBittorrent names differ by case, and the select needs the saved one itself.
		return cur && !cats.includes(cur) ? [cur, ...cats] : cats;
	}
	// A change that could not be kept goes back, so the page never shows an unsaved one.
	async function keep(k: 'movies' | 'shows' | 'music', field: 'category_movies' | 'category_shows' | 'category_music', value: string, before: string) {
		if (!save) return;
		try {
			await api.put(`/downloaders/${id}`, { name, [field]: value });
		} catch (err) {
			if (get(k) === value) set(k, before);
			toasts.push(message(err), 'fail');
		}
	}
	const kinds = $derived([
		{ key: 'movies', kind: 'movie', field: 'category_movies', label: t('settings.downloaders.categoryMovies') },
		{ key: 'shows', kind: 'show', field: 'category_shows', label: t('settings.downloaders.categoryShows') },
		{ key: 'music', kind: 'artist', field: 'category_music', label: t('settings.downloaders.categoryMusic') }
	] as const);
	function get(k: 'movies' | 'shows' | 'music'): string {
		return k === 'movies' ? movies : k === 'shows' ? shows : music;
	}
	function set(k: 'movies' | 'shows' | 'music', v: string) {
		if (k === 'movies') movies = v;
		else if (k === 'shows') shows = v;
		else music = v;
	}
</script>

{#if !id}
	<p class="hint" data-testid="dl-cat-picked">{t('settings.downloaders.categoriesPicked')}</p>
{:else if listed}
	<div class="kinds" class:row={save} data-testid="dl-kind-categories">
		{#each kinds as k (k.key)}
			<Field id="{pre}-{k.key}" label={k.label}>
				<select id="{pre}-{k.key}" class="input" value={get(k.key)} onchange={(e) => { const v = (e.currentTarget as HTMLSelectElement).value; const before = get(k.key); set(k.key, v); void keep(k.key, k.field, v, before); }}>
					<option value="">{t('settings.downloaders.sameAsMain', { name: main })}</option>
					{#each options(get(k.key)) as c (c)}<option value={c}>{c}</option>{/each}
				</select>
			</Field>
		{/each}
	</div>
{:else if listed === false && !save}
	<div class="kinds">
		{#each kinds as k (k.key)}
			<Field id="{pre}-{k.key}" label={k.label} hint={t('settings.downloaders.categoryKindHint')}>
				<input id="{pre}-{k.key}" class="input" value={get(k.key)} oninput={(e) => set(k.key, (e.currentTarget as HTMLInputElement).value)} maxlength="64" aria-describedby="{pre}-{k.key}-hint" />
			</Field>
		{/each}
	</div>
{/if}

<style>
	.kinds { margin-top: var(--s-4); }
	.row { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: var(--s-3); }
	.row :global(.field + .field) { margin-top: 0; }
	.hint { margin-top: var(--s-4); }
	@media (max-width: 640px) { .row { grid-template-columns: 1fr; } }
</style>
