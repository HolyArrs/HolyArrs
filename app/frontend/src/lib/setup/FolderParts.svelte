<script lang="ts">
	// Setup's Folders step (Start fresh): a count tile per part on top, and under it a column per part with each of its folders:
	// every folder shows what HolyArrs found in it and can be changed or removed, and + Add folder adds another (picked or typed).
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { api, message } from '#lib/core/api.ts';
	import Icon, { type IconName } from '#lib/ui/Icon.svelte';
	import FolderField from '#lib/ui/FolderField.svelte';

	type Part = { kind: string; label: string; icon: IconName; hint: string; path: string };
	type Saved = { id: number; kind: string; path: string };
	// off: the parts turned off; + adds one here (Music without Lidarr, say), and its folder box opens.
	let { parts, off = [], busy = false, onchange, onturnon }: { parts: Part[]; off?: { part: string; label: string; icon: IconName }[]; busy?: boolean; onchange?: () => void; onturnon?: (part: string) => void } = $props();

	let saved = $state<Saved[]>([]);
	let counts = $state<Record<string, number>>({});
	// What is being typed: a new folder per part, or the new path for a folder being changed (by id).
	let adding = $state<Record<string, string>>({});
	let editing = $state<Record<number, string>>({});
	let errors = $state<Record<string, string>>({});
	let working = $state(false);

	async function load() {
		saved = await api.get<{ library_folders: Saved[] }>('/library-folders').then((r) => r.library_folders ?? [], () => []);
		for (const f of saved) count(f.path);
		// A part with no folder yet starts with its box open, filled with the folder found for it.
		for (const p of parts) if (!saved.some((f) => f.kind === p.kind) && adding[p.kind] === undefined) adding = { ...adding, [p.kind]: p.path };
	}
	function count(path: string) {
		if (path in counts) return;
		api.post<{ folders: unknown[] }>('/migrate/folders', { path, names: [] }).then((r) => (counts = { ...counts, [path]: r.folders.length }), () => (counts = { ...counts, [path]: -1 }));
	}
	$effect(() => {
		void parts.length;
		void load();
	});

	async function add(p: Part) {
		const path = (adding[p.kind] ?? '').trim();
		if (!path || working) return;
		working = true;
		errors = { ...errors, [p.kind]: '' };
		try {
			await api.post('/library-folders', { path, kind: p.kind, label: p.label });
			const { [p.kind]: _, ...rest } = adding;
			adding = rest;
			await load();
			onchange?.();
		} catch (err) {
			errors = { ...errors, [p.kind]: message(err) };
		} finally {
			working = false;
		}
	}
	async function change(f: Saved, p: Part) {
		const path = (editing[f.id] ?? '').trim();
		if (!path || working) return;
		if (path === f.path) return void stopEdit(f.id);
		working = true;
		errors = { ...errors, [f.id]: '' };
		try {
			await api.del(`/library-folders/${f.id}`);
			try {
				await api.post('/library-folders', { path, kind: f.kind, label: p.label });
			} catch (err) {
				await api.post('/library-folders', { path: f.path, kind: f.kind, label: p.label }).catch(() => undefined);
				throw err;
			}
			stopEdit(f.id);
			await load();
			onchange?.();
		} catch (err) {
			errors = { ...errors, [f.id]: words(err) };
		} finally {
			working = false;
		}
	}
	async function remove(f: Saved) {
		if (working) return;
		working = true;
		try {
			await api.del(`/library-folders/${f.id}`);
			await load();
			onchange?.();
		} catch (err) {
			errors = { ...errors, [f.id]: words(err) };
		} finally {
			working = false;
		}
	}
	// The server's refusals in plain words: a folder that already holds imported titles stays.
	function words(err: unknown): string {
		const m = message(err);
		return /titles still use/i.test(m) ? t('welcome.folders.inUse') : m;
	}
	function stopEdit(id: number) {
		const { [id]: _, ...rest } = editing;
		editing = rest;
	}
	const word = (k: string, n: number) => t(k === 'movie' ? (n === 1 ? 'migrate.parts.movie' : 'migrate.parts.movies') : k === 'show' ? (n === 1 ? 'migrate.parts.show' : 'migrate.parts.shows') : n === 1 ? 'migrate.parts.artist' : 'migrate.parts.artists');
	const listOf = (k: string) => saved.filter((f) => f.kind === k);
	const total = (k: string) => listOf(k).reduce((a, f) => a + Math.max(0, counts[f.path] ?? 0), 0);
</script>

<div class="fp" aria-busy={working} style="--cols: repeat({parts.length}, minmax(0, 1fr)){off.length && onturnon ? ` repeat(${off.length}, minmax(140px, 0.45fr))` : ''}">
	<div class="tiles" data-testid="folder-stats">
		{#each parts as p (p.kind)}
			<div class="tile"><span class="ic" aria-hidden="true"><Icon name={p.icon} size={18} /></span><b class="big">{total(p.kind)}</b><span class="sub">{word(p.kind, total(p.kind))} · {t(listOf(p.kind).length === 1 ? 'welcome.folders.oneFolder' : 'welcome.folders.nFolders', { n: listOf(p.kind).length })}</span></div>
		{/each}
		{#if onturnon}{#each off as o (o.part)}<button type="button" class="tile off" disabled={busy || working} onclick={() => onturnon(o.part)} data-testid="folder-part-{o.part}"><span class="ic" aria-hidden="true"><Icon name="plus" size={18} /></span><b class="small">{o.label}</b><span class="sub">{t('welcome.folders.addPart')}</span></button>{/each}{/if}
	</div>
	<div class="cols">
		{#each parts as p (p.kind)}
			<section class="col" aria-label={p.label}>
				<h3 class="h"><Icon name={p.icon} size={18} />{p.label}</h3>
				{#each listOf(p.kind) as f (f.id)}
					{#if editing[f.id] !== undefined}
						<FolderField id="folder-{f.id}" label={p.label} kind={p.kind} hint={p.hint} bind:value={editing[f.id]} error={errors[f.id] ?? ''} disabled={busy || working} onsave={() => change(f, p)} oncancel={() => stopEdit(f.id)} />
					{:else}
						<div class="saved" data-testid="folder-saved">
							<span class="path" dir="ltr">{f.path}</span>
							<span class="n" class:bad={counts[f.path] === -1}>{counts[f.path] === undefined ? '…' : counts[f.path] === -1 ? t('migrate.browse.cantSee') : t('migrate.someFound', { n: counts[f.path] })}</span>
							<button type="button" class="q" disabled={busy || working} onclick={() => (editing = { ...editing, [f.id]: f.path })}>{t('welcome.change')}</button>
							<button type="button" class="q" disabled={busy || working} onclick={() => remove(f)} aria-label={t('welcome.folders.remove', { path: f.path })}><Icon name="x" size={16} /></button>
							{#if onturnon}{#each off as o (o.part)}<span class="gap" aria-hidden="true"></span>{/each}{/if}
	</div>
						{#if errors[f.id]}<p class="form-error" role="alert">{errors[f.id]}</p>{/if}
					{/if}
				{/each}
				{#if adding[p.kind] !== undefined}
					<FolderField id="folder-new-{p.kind}" label={p.label} kind={p.kind} hint={p.hint} bind:value={adding[p.kind]} error={errors[p.kind] ?? ''} disabled={busy || working} onsave={() => add(p)} oncancel={listOf(p.kind).length ? () => { const { [p.kind]: _, ...rest } = adding; adding = rest; } : undefined} />
				{:else}
					<button type="button" class="addf" disabled={busy || working} onclick={() => (adding = { ...adding, [p.kind]: '' })} data-testid="folder-add-{p.kind}"><Icon name="plus" size={16} />{t('welcome.folders.addFolder')}</button>
				{/if}
			</section>
		{/each}
	</div>
</div>

<style>
	.fp { display: flex; flex-direction: column; gap: 14px; }
	.tiles, .cols { display: grid; grid-template-columns: var(--cols); gap: 12px; }
	@media (max-width: 700px) { .tiles, .cols { grid-template-columns: 1fr; } .gap { display: none; } }
	.tile { display: grid; grid-template-columns: auto 1fr; grid-template-rows: auto auto; column-gap: 10px; align-items: center; padding: 14px 16px; border-radius: 16px; border: 1px solid var(--g-line, var(--line)); background: var(--g-bg, var(--surface-1)); }
	.tile .ic { grid-row: 1 / 3; color: var(--text-muted); }
	.big { font: 700 28px/1 var(--font-ui); color: var(--text); font-variant-numeric: tabular-nums; }
	.sub { font: var(--t-footnote); color: var(--text-muted); }
	.col { display: flex; flex-direction: column; gap: 8px; min-width: 0; }
	.h { margin: 0 0 2px; display: flex; align-items: center; gap: 8px; font: 600 16px/1.2 var(--font-ui); color: var(--text); }
	.saved { display: flex; align-items: center; gap: 8px; min-height: 44px; padding: 0 4px 0 12px; border-radius: 12px; border: 1px solid var(--g-line, var(--line)); background: var(--g-bg, var(--surface-1)); min-width: 0; }
	.path { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text); font: 400 14px/1.3 var(--font-ui); }
	.n { flex: none; font: 600 12px/1 var(--font-ui); color: var(--ok-text); white-space: nowrap; }
	.n.bad { color: var(--bad-text, #ff6b6b); }
	.q { flex: none; min-height: 36px; padding: 0 8px; border: 0; border-radius: 8px; background: none; color: var(--text-muted); font: 500 13px/1 var(--font-ui); cursor: pointer; display: inline-flex; align-items: center; }
	.q:hover:not(:disabled) { color: var(--text); background: color-mix(in srgb, var(--text) 8%, transparent); }
	.addf { align-self: flex-start; display: inline-flex; align-items: center; gap: 6px; min-height: 40px; padding: 0 12px; border-radius: 999px; border: 1px dashed var(--g-line-hover, var(--line-strong)); background: none; color: var(--text); font: 600 14px/1 var(--font-ui); cursor: pointer; }
	.tile.off { border-style: dashed; background: none; color: var(--text); cursor: pointer; text-align: start; font: inherit; }
	.tile.off:hover:not(:disabled) { background: color-mix(in srgb, var(--text) 6%, transparent); }
	.tile.off:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.small { font: 600 16px/1.2 var(--font-ui); color: var(--text); }
	.q:focus-visible, .addf:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.q:disabled, .addf:disabled { opacity: 0.5; cursor: default; }
</style>
