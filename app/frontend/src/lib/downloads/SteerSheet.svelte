<script lang="ts">
	// One sheet for steering one or many torrents by hand: seeding rules, a new folder or speed caps. One call per Downloader.
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { byClient, bytesOf, checkLimits, kbOf, modeOf, valueOf, type LimitMode, type ShareLimits, type SteerMode } from './manage.ts';
	import { api, message } from '../core/api.ts';
	import { toasts } from '../core/toasts.svelte.ts';
	import type { DownloadRow } from '../core/types.ts';

	let { open = $bindable(false), mode, rows, ondone }: { open?: boolean; mode: SteerMode; rows: DownloadRow[]; ondone?: () => void } = $props();

	let busy = $state(false);
	let error = $state('');
	// Each limit is the Downloader's rule, none, or a value.
	let ratioMode = $state<LimitMode>('global');
	let ratio = $state(2);
	let seedMode = $state<LimitMode>('global');
	let seedMin = $state(1440);
	let idleMode = $state<LimitMode>('global');
	let idleMin = $state(60);
	let action = $state<ShareLimits['action']>('');
	// A subfolder (full paths need settings access), optionally under a category's folder.
	let category = $state('');
	let folder = $state('');
	// In KB/s; 0 is no cap.
	let down = $state(0);
	let up = $state(0);
	const TITLE: Record<SteerMode, string> = { seed: 'Seeding rules', location: 'Set location', speed: 'Speed limits' };

	const groups = $derived(byClient(rows));
	const count = $derived([...groups.values()].reduce((k, g) => k + g.length, 0));
	const noun = $derived(count === 1 ? 'this torrent' : `${count} torrents`);

	// Each opening starts from the first torrent's own rules.
	let wasOpen = false;
	$effect(() => {
		if (open && !wasOpen) {
			error = '';
			const l = rows.find((r) => r.limits)?.limits;
			ratioMode = modeOf(l?.ratio ?? -2);
			ratio = l && l.ratio >= 0 ? l.ratio : 2;
			seedMode = modeOf(l?.seeding_minutes ?? -2);
			seedMin = l && l.seeding_minutes >= 0 ? l.seeding_minutes : 1440;
			idleMode = modeOf(l?.inactive_minutes ?? -2);
			idleMin = l && l.inactive_minutes >= 0 ? l.inactive_minutes : 60;
			action = l?.action ?? '';
			category = rows.length === 1 ? (rows[0].category ?? '') : '';
			folder = '';
			down = rows.length === 1 ? kbOf(rows[0].limit_down) : 0;
			up = rows.length === 1 ? kbOf(rows[0].limit_up) : 0;
		}
		wasOpen = open;
	});

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (mode === 'location') return send('location', { category: category.trim(), folder: folder.trim() }, `Moving ${noun}`, !folder.trim() ? 'Type a folder.' : '');
		if (mode === 'speed') return send('speed-limits', { down: bytesOf(down), up: bytesOf(up) }, `Speed limits set for ${noun}`, '');
		const l: ShareLimits = { ratio: valueOf(ratioMode, Number(ratio)), seeding_minutes: valueOf(seedMode, Number(seedMin)), inactive_minutes: valueOf(idleMode, Number(idleMin)), action };
		error = checkLimits(l);
		if (error || !count) return;
		busy = true;
		const bad: string[] = [];
		for (const [id, list] of groups) {
			try {
				await api.post(`/downloaders/${id}/share-limits`, { hashes: list.map((r) => r.hash), ...l });
			} catch (err) {
				bad.push(message(err));
			}
		}
		busy = false;
		if (bad.length) {
			error = bad.join(' · ');
			return;
		}
		toasts.push(`Seeding rules set for ${noun}`, 'ok');
		open = false;
		ondone?.();
	}
	async function send(path: string, body: Record<string, unknown>, done: string, wrong: string) {
		error = wrong;
		if (error || !count) return;
		busy = true;
		const bad: string[] = [];
		for (const [id, list] of groups) {
			try {
				await api.post(`/downloaders/${id}/${path}`, { hashes: list.map((r) => r.hash), ...body });
			} catch (err) {
				bad.push(message(err));
			}
		}
		busy = false;
		if (bad.length) {
			error = bad.join(' · ');
			return;
		}
		toasts.push(done, 'ok');
		open = false;
		ondone?.();
	}
	const MODES: { v: LimitMode; label: string }[] = [{ v: 'global', label: "Downloader's rule" }, { v: 'none', label: 'No limit' }, { v: 'set', label: 'Set' }];
</script>

{#snippet limit(label: string, unit: string, testid: string, getMode: () => LimitMode, setMode: (m: LimitMode) => void, getV: () => number, setV: (v: number) => void, step: string)}
	<fieldset class="lim" data-testid="steer-{testid}">
		<legend class="lb">{label}</legend>
		<div class="seg" role="radiogroup" aria-label={label}>
			{#each MODES as m (m.v)}<button type="button" role="radio" aria-checked={getMode() === m.v} class:on={getMode() === m.v} onclick={() => setMode(m.v)} data-testid="steer-{testid}-{m.v}">{m.label}</button>{/each}
		</div>
		{#if getMode() === 'set'}
			<label class="val"><input type="number" min="0" {step} value={getV()} oninput={(e) => setV(Number(e.currentTarget.value))} aria-label="{label} value" data-testid="steer-{testid}-value" /><span>{unit}</span></label>
		{/if}
	</fieldset>
{/snippet}

<Sheet bind:open title={TITLE[mode]} size="sm">
	<form id="steer-form" class="sf" onsubmit={submit} data-testid="steer-form" data-mode={mode}>
	{#if mode === 'location'}
		<p class="lead">Moves the files of {noun} on disk. The Downloader makes the folder if it is missing.</p>
		<label class="fl">
			<span class="lb">Category folder <span class="opt">optional</span></span>
			<input type="text" bind:value={category} placeholder="None: the Downloader's own folder" spellcheck="false" data-testid="steer-category" />
		</label>
		<label class="fl">
			<span class="lb">Folder</span>
			<input type="text" bind:value={folder} placeholder="A subfolder, like tv/old (full paths need settings access)" spellcheck="false" data-testid="steer-folder" />
		</label>
	{:else if mode === 'speed'}
		<p class="lead">Caps {noun} on its own, under the Downloader's overall limits. 0 means no limit.</p>
		<div class="two">
			<label class="fl"><span class="lb">Download</span><span class="val"><input type="number" min="0" step="1" bind:value={down} data-testid="steer-down" /><span>KB/s</span></span></label>
			<label class="fl"><span class="lb">Upload</span><span class="val"><input type="number" min="0" step="1" bind:value={up} data-testid="steer-up" /><span>KB/s</span></span></label>
		</div>
	{:else}
		<p class="lead">For {noun}. When any limit is reached, the Downloader acts.</p>
		{@render limit('Share ratio', '× uploaded', 'ratio', () => ratioMode, (m) => (ratioMode = m), () => ratio, (v) => (ratio = v), '0.1')}
		{@render limit('Seeding time', 'minutes', 'seed', () => seedMode, (m) => (seedMode = m), () => seedMin, (v) => (seedMin = v), '1')}
		{@render limit('Idle time', 'minutes with no upload', 'idle', () => idleMode, (m) => (idleMode = m), () => idleMin, (v) => (idleMin = v), '1')}
		<label class="fl">
			<span class="lb">Then</span>
			<select bind:value={action} data-testid="steer-action">
				<option value="">Downloader's default</option>
				<option value="stop">Stop</option>
				<option value="remove">Remove (keep files)</option>
				{#if action === 'remove_files'}<option value="remove_files">Remove with files</option>{/if}
				{#if action === 'super_seed'}<option value="super_seed">Super seed</option>{/if}
			</select>
		</label>
	{/if}
		{#if error}<p class="err" role="alert">{error}</p>{/if}
	</form>
	{#snippet footer()}
		<Button variant="plain" onclick={() => (open = false)}>Cancel</Button>
		<Button variant="primary" type="submit" form="steer-form" {busy} disabled={busy || !count} data-testid="steer-submit">{mode === 'location' ? 'Move' : 'Save'}</Button>
	{/snippet}
</Sheet>

<style>
	.sf { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
	.lead { margin: 0; color: var(--text-muted); font: var(--t-callout); }
	.lim { border: 0; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; min-width: 0; }
	.lb { font: var(--t-subhead); color: var(--text-muted); padding: 0; }
	.fl { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
	.seg { display: flex; flex-wrap: wrap; gap: 4px; padding: 3px; border-radius: 10px; background: var(--surface-2); border: 1px solid var(--line-strong); }
	.seg button { flex: 1 1 auto; min-height: 36px; padding: 0 10px; border: 0; border-radius: 8px; background: none; color: var(--text-muted); font: var(--t-callout); cursor: pointer; }
	.seg button.on { background: var(--seg-thumb); color: var(--text); box-shadow: var(--shadow-1, 0 1px 2px rgb(0 0 0 / 0.12)); }
	.seg button:focus-visible, input:focus-visible, select:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.val { display: flex; align-items: center; gap: 10px; color: var(--text-muted); font: var(--t-callout); min-width: 0; }
	.opt { opacity: 0.75; }
	.two { display: flex; flex-wrap: wrap; gap: 12px; }
	.two .fl { flex: 1 1 140px; }
	input[type='text'] { width: 100%; box-sizing: border-box; min-height: 44px; padding: 10px 12px; border-radius: 10px; border: 1px solid var(--line-strong); background: var(--surface-2); color: var(--text); font: var(--t-callout); }
	input[type='text']:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	input[type='number'], select { box-sizing: border-box; min-height: 44px; padding: 10px 12px; border-radius: 10px; border: 1px solid var(--line-strong); background: var(--surface-2); color: var(--text); font: var(--t-callout); }
	input[type='number'] { width: 120px; }
	select { width: 100%; }
	.err { color: var(--fail-text); font: var(--t-callout); margin: 0; }
</style>
