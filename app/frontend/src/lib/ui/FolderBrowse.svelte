<script lang="ts">
	// Setup's Folders step: walk the folders HolyArrs can see, pick one, and see how many of the old app's titles each holds.
	import { ltr, t } from '#lib/core/i18n/i18n.svelte.ts';
	import { api } from '#lib/core/api.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import Button from '#lib/ui/Button.svelte';

	let { names, title, hint = '', current = '', onpick, oncancel }: { names: string[]; title: string; hint?: string; current?: string; onpick: (path: string) => void; oncancel: () => void } = $props();

	type Entry = { path: string; found: number };
	let at = $state('');
	let list = $state<Entry[]>([]);
	let checked = $state(0);
	let loading = $state(true);
	let failed = $state('');
	let chosen = $state('');

	const parent = (p: string) => (p.split('/').filter(Boolean).length > 1 ? p.slice(0, p.lastIndexOf('/')) : '');
	async function open(p: string) {
		loading = true;
		failed = '';
		try {
			const r = await api.post<{ path: string; folders: Entry[]; checked: number }>('/migrate/folders', { path: p, names });
			at = r.path;
			list = r.folders;
			checked = r.checked;
			chosen = list.some((e) => e.path === chosen) ? chosen : '';
		} catch (e) {
			failed = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
	}
	// Opens where the current folder is, with it chosen.
	$effect(() => {
		chosen = current;
		void open(current ? parent(current) : '');
	});
	const crumbs = $derived(at ? at.split('/').filter(Boolean).map((_, i, a) => '/' + a.slice(0, i + 1).join('/')) : []);
	const leaf = (p: string) => p.split('/').filter(Boolean).pop() ?? p;
	const pickPath = $derived(chosen || at);
</script>

<div class="fb" data-testid="folder-browse">
	<header class="fb-head">
		<h2 class="fb-title">{title}</h2>
		{#if hint}<p class="fb-hint" dir="auto">{hint}</p>{/if}
	</header>
	<nav class="fb-crumbs" aria-label={t('migrate.browse.start')}>
		<button type="button" class="crumb" class:on={!at} onclick={() => open('')}><Icon name="home" size={15} /><span class="sr-only">{t('migrate.browse.start')}</span></button>
		{#each crumbs as c (c)}<Icon name="chevronRight" size={13} /><button type="button" class="crumb" class:on={c === at} onclick={() => open(c)} dir="ltr">{leaf(c)}</button>{/each}
	</nav>
	<div class="fb-list" aria-busy={loading}>
		{#if failed}<p class="form-error" role="alert">{failed}</p>
		{:else if loading && list.length === 0}{#each [0, 1, 2] as i (i)}<div class="row ghost" aria-hidden="true"></div>{/each}
		{:else if list.length === 0}<p class="fb-empty">{t('migrate.browse.empty')}</p>
		{:else}
			{#each list as e (e.path)}
				<div class="row" class:on={chosen === e.path}>
					<button type="button" class="pick" aria-pressed={chosen === e.path} onclick={() => (chosen = e.path)}>
						<span class="ic" aria-hidden="true"><Icon name={chosen === e.path ? 'check' : 'folder'} size={16} /></span>
						<span class="name" dir="ltr">{at ? leaf(e.path) : ltr(e.path)}</span>
						{#if checked && e.found}<span class="hit" class:all={e.found * 5 >= checked * 4}>{e.found === checked ? t('migrate.allHere') : e.found * 5 >= checked * 4 ? t('migrate.nFound', { found: e.found }) : t('migrate.someHere', { found: e.found, checked })}</span>{/if}
					</button>
					<button type="button" class="in" aria-label={leaf(e.path)} onclick={() => open(e.path)}><Icon name="chevronRight" size={16} /></button>
				</div>
			{/each}
		{/if}
	</div>
	<footer class="fb-foot">
		<span class="fb-path mono" dir="ltr">{pickPath ? ltr(pickPath) : ''}</span>
		<Button variant="secondary" onclick={oncancel}>{t('migrate.browse.close')}</Button>
		<Button variant="primary" disabled={!pickPath} onclick={() => onpick(pickPath)} data-testid="folder-use">{t('migrate.browse.use')}</Button>
	</footer>
</div>

<style>
	.fb { display: flex; flex-direction: column; gap: 14px; }
	.fb-head { text-align: center; display: flex; flex-direction: column; gap: 4px; }
	.fb-title { margin: 0; font: 700 22px/1.2 var(--font-ui); letter-spacing: -0.01em; color: var(--text, #fff); }
	.fb-hint { margin: 0; font: var(--t-footnote); color: var(--text-muted); }
	.fb-crumbs { display: flex; align-items: center; flex-wrap: wrap; gap: 2px; color: var(--text-muted); }
	.crumb { display: inline-flex; align-items: center; min-height: 32px; padding: 0 10px; border: 0; border-radius: 999px; background: none; color: var(--text-muted); font: 500 14px/1 var(--font-ui); cursor: pointer; }
	.crumb.on { color: var(--text, #fff); background: rgb(var(--ink-rgb, 255 255 255) / 0.08); }
	.crumb:hover { color: var(--text, #fff); }
	.fb-list { display: flex; flex-direction: column; gap: 6px; max-height: min(46vh, 340px); overflow-y: auto; padding: 2px; }
	.row { display: flex; align-items: stretch; min-height: 50px; border-radius: 12px; border: 1px solid var(--g-line, rgb(255 255 255 / 0.1)); background: var(--g-bg, rgb(255 255 255 / 0.05)); transition: border-color var(--dur-fast, 120ms), background var(--dur-fast, 120ms); }
	.row.on { border-color: color-mix(in srgb, var(--accent, #e53935) 70%, transparent); background: color-mix(in srgb, var(--accent, #e53935) 12%, transparent); }
	@media (prefers-reduced-motion: no-preference) { .row.ghost { animation: fb-ghost 1.2s ease-in-out infinite alternate; } }
	@keyframes fb-ghost { to { opacity: 0.45; } }
	.pick { flex: 1; min-width: 0; display: flex; align-items: center; gap: 10px; padding: 0 14px; border: 0; background: none; color: var(--text, #fff); font: 500 15px/1.2 var(--font-ui); text-align: start; cursor: pointer; border-start-start-radius: 12px; border-end-start-radius: 12px; }
	.ic { display: inline-flex; color: var(--text-muted); }
	.row.on .ic { color: var(--accent, #e53935); }
	.name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.hit { flex: none; font: 600 12px/1 var(--font-ui); padding: 4px 9px; border-radius: 999px; color: var(--warn-text, #f5a524); background: color-mix(in srgb, var(--warn-text, #f5a524) 16%, transparent); }
	.hit.all { color: var(--ok-text); background: color-mix(in srgb, var(--ok-text) 16%, transparent); }
	.in { flex: none; width: 48px; display: grid; place-items: center; border: 0; border-inline-start: 1px solid var(--g-line, rgb(255 255 255 / 0.1)); background: none; color: var(--text-muted); cursor: pointer; border-start-end-radius: 12px; border-end-end-radius: 12px; }
	.row:hover:not(.on) { background: rgb(var(--ink-rgb, 255 255 255) / 0.09); border-color: rgb(var(--ink-rgb, 255 255 255) / 0.18); }
	.in:hover { color: var(--text, #fff); background: rgb(var(--ink-rgb, 255 255 255) / 0.06); }
	.crumb:focus-visible, .pick:focus-visible, .in:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	:global([dir='rtl']) .in :global(svg), :global([dir='rtl']) .fb-crumbs :global(svg) { transform: scaleX(-1); }
	.fb-empty { margin: 12px 0; text-align: center; color: var(--text-muted); font: var(--t-footnote); }
	.fb-foot { display: flex; align-items: center; gap: 10px; padding-top: 4px; }
	.fb-path { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); font-size: 13px; }
	@media (max-width: 600px) { .fb-foot { flex-wrap: wrap; } .fb-path { flex-basis: 100%; } }
</style>
