<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// Add sheet: magnet or .torrent links, or dropped .torrent files, sent straight to the chosen Downloader with its category, folder and paused choice.
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import { addWords, isTorrentFile, splitLinks, type AddOutcome } from './manage.ts';
	import { api, message } from '../core/api.ts';
	import { toasts } from '../core/toasts.svelte.ts';

	type Client = { id: number; name: string; category: string };
	let { open = $bindable(false), clients, ondone }: { open?: boolean; clients: Client[]; ondone?: () => void } = $props();

	let text = $state('');
	let files = $state<File[]>([]);
	let clientId = $state(0);
	let category = $state('');
	let savePath = $state('');
	let paused = $state(false);
	let busy = $state(false);
	let error = $state('');
	let over = $state(false);
	let picker = $state<HTMLInputElement>();

	const parsed = $derived(splitLinks(text));
	const count = $derived(parsed.links.length + files.length);
	const client = $derived(clients.find((c) => c.id === clientId));

	// Starts clean on the first Downloader each time the sheet opens.
	let wasOpen = false;
	$effect(() => {
		if (open && !wasOpen) {
			text = '';
			files = [];
			error = '';
			paused = false;
			savePath = '';
			clientId = clients[0]?.id ?? 0;
			category = clients[0]?.category ?? '';
		}
		wasOpen = open;
	});
	function pickClient(id: number) {
		clientId = id;
		category = clients.find((c) => c.id === id)?.category ?? '';
	}
	function addFiles(list: FileList | File[] | null | undefined) {
		const got = [...(list ?? [])];
		const ok = got.filter(isTorrentFile);
		if (ok.length < got.length) error = got.length - ok.length === 1 ? t('downloads.at.oneNot') : t('downloads.at.manyNot', { count: got.length - ok.length });
		else error = '';
		files = [...files, ...ok.filter((f) => !files.some((x) => x.name === f.name && x.size === f.size))];
	}
	function drop(e: DragEvent) {
		e.preventDefault();
		over = false;
		addFiles(e.dataTransfer?.files);
	}
	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (parsed.bad.length) {
			error = t('downloads.at.notLink', { link: parsed.bad[0] });
			return;
		}
		if (!count || !client) return;
		busy = true;
		error = '';
		try {
			const fd = new FormData();
			fd.set('links', parsed.links.join('\n'));
			fd.set('category', category.trim());
			fd.set('save_path', savePath.trim());
			fd.set('paused', String(paused));
			for (const f of files) fd.append('torrents', f, f.name);
			const out = await api.request<AddOutcome>(`/downloaders/${client.id}/torrents`, { method: 'POST', raw: fd });
			const w = addWords(out);
			toasts.push(w.text, w.tone === 'ok' ? 'ok' : w.tone === 'fail' ? 'fail' : 'info', { body: w.body });
			if (w.tone !== 'fail') {
				open = false;
				ondone?.();
			} else error = w.text;
		} catch (err) {
			error = message(err);
		} finally {
			busy = false;
		}
	}
</script>

<Sheet bind:open title={t('downloads.at.title')} size="md">
	<form id="add-form" class="af" onsubmit={submit} data-testid="dl-add-form">
		<label class="fl">
			<span class="lb">{t('downloads.at.links')}</span>
			<textarea rows="4" bind:value={text} placeholder={t('downloads.at.linksPh')} spellcheck="false" data-testid="dl-add-links"></textarea>
		</label>
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="drop" class:over ondragover={(e) => { e.preventDefault(); over = true; }} ondragleave={() => (over = false)} ondrop={drop} data-testid="dl-add-drop">
			<p>{t('downloads.at.drop1')} <button type="button" class="lk" onclick={() => picker?.click()}>{t('downloads.at.choose')}</button></p>
			<input bind:this={picker} type="file" accept=".torrent,application/x-bittorrent" multiple hidden onchange={(e) => { addFiles(e.currentTarget.files); e.currentTarget.value = ''; }} data-testid="dl-add-file" />
			{#if files.length}
				<ul class="fs">
					{#each files as f, i (f.name + f.size)}
						<li><span class="fn" title={f.name}>{f.name}</span><button type="button" class="lk" aria-label={t('downloads.at.removeFile', { name: f.name })} onclick={() => (files = files.filter((_, j) => j !== i))}>{t('downloads.at.remove')}</button></li>
					{/each}
				</ul>
			{/if}
		</div>
		<div class="row2">
			<label class="fl">
				<span class="lb">{t('downloads.at.downloader')}</span>
				<select value={clientId} onchange={(e) => pickClient(Number(e.currentTarget.value))} data-testid="dl-add-client">
					{#each clients as c (c.id)}<option value={c.id}>{c.name}</option>{/each}
				</select>
			</label>
			<label class="fl">
				<span class="lb">{t('downloads.at.category')}</span>
				<input type="text" bind:value={category} placeholder={t('downloads.at.none')} data-testid="dl-add-category" />
			</label>
		</div>
		<label class="fl">
			<span class="lb">{t('downloads.at.saveIn')} <span class="opt">{t('downloads.at.optional')}</span></span>
			<input type="text" bind:value={savePath} placeholder={t('downloads.at.savePh')} spellcheck="false" data-testid="dl-add-path" />
		</label>
		<label class="ck"><input type="checkbox" bind:checked={paused} data-testid="dl-add-paused" /><span>{t('downloads.at.paused')}</span></label>
		{#if error}<p class="err" role="alert">{error}</p>{/if}
	</form>
	{#snippet footer()}
		<Button variant="plain" onclick={() => (open = false)}>{t('downloads.sel.cancel')}</Button>
		<Button variant="primary" type="submit" form="add-form" {busy} disabled={busy || !count || !client} data-testid="dl-add-submit">{count > 1 ? t('downloads.at.addN', { count }) : t('downloads.at.add')}</Button>
	{/snippet}
</Sheet>

<style>
	.af { display: flex; flex-direction: column; gap: 14px; min-width: 0; }
	.fl { display: flex; flex-direction: column; gap: 6px; min-width: 0; flex: 1 1 160px; }
	.lb { font: var(--t-subhead); color: var(--text-muted); }
	.opt { opacity: 0.75; }
	textarea, input[type='text'], select { width: 100%; box-sizing: border-box; min-height: 44px; padding: 10px 12px; border-radius: 10px; border: 1px solid var(--line-strong); background: var(--surface-2); color: var(--text); font: var(--t-callout); }
	textarea { resize: vertical; font-family: var(--font-mono); font-size: 13px; }
	textarea:focus-visible, input:focus-visible, select:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.row2 { display: flex; flex-wrap: wrap; gap: 12px; }
	.drop { border: 1px dashed var(--line-strong); border-radius: 12px; padding: 14px; color: var(--text-muted); font: var(--t-callout); text-align: center; transition: background-color 160ms var(--ease-out), border-color 160ms var(--ease-out); }
	.drop.over { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 8%, transparent); }
	.drop p { margin: 0; }
	.fs { list-style: none; margin: 10px 0 0; padding: 0; display: flex; flex-direction: column; gap: 6px; text-align: left; }
	.fs li { display: flex; align-items: center; justify-content: space-between; gap: 10px; min-width: 0; color: var(--text); }
	.fn { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; }
	.lk { background: none; border: 0; padding: 0; color: var(--accent-text); font: inherit; font-weight: 500; cursor: pointer; min-height: 24px; }
	.lk:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 4px; }
	.ck { display: flex; gap: 10px; align-items: center; font: var(--t-callout); cursor: pointer; }
	.ck input { width: 20px; height: 20px; accent-color: var(--accent); }
	.err { color: var(--fail-text); font: var(--t-callout); margin: 0; }
</style>
