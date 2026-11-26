<script lang="ts">
	// Downloads in first-run setup: the connected downloaders with their test result, and the form that adds or edits one.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import TestTag from '#lib/ui/TestTag.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import { KindCategories, kindWords, BroughtKey } from '#lib/downloads/index.ts';
	import type { createWelcomeDownloader } from './welcome-downloader.svelte.ts';

	import IconButton from '#lib/ui/IconButton.svelte';
	import { untrack } from 'svelte';
	import { api, message } from '#lib/core/api.ts';
	import { joinDlAddress, splitDlAddress, sabKeyPage } from './dl-address.ts';

	let { downloaderStep, busy, appName, error = $bindable() }: { appName: (k: string) => string; downloaderStep: ReturnType<typeof createWelcomeDownloader>; busy: boolean; error: string } = $props();

	// What connecting does, shown beside the form (i18n keys, {name} is the app).
	const DL_BRINGS = ['welcome.dl.brings.sends', 'welcome.dl.brings.categories', 'welcome.dl.brings.progress', 'welcome.dl.brings.imports'];
	// The address as Protocol | host | Port; each change writes the full address back to the form.
	let ssl = $state(false);
	let host = $state('');
	let port = $state('');
	let basePath = $state('');
	// Addresses the network search found for the picked app, and which one is picked.
	let found = $state<string[]>([]);
	let pick = $state('manual');
	let finding = $state(false);
	let findNote = $state('');
	// A new form (another app, Edit, a reset) fills the boxes from its address.
	$effect(() => {
		const d = downloaderStep.dl;
		const k = d.kind;
		untrack(() => {
			({ ssl, host, port, basePath } = splitDlAddress(d.url, downloaderStep.dlPorts[k]));
			found = [];
			pick = 'manual';
			findNote = '';
		});
	});
	function typed() {
		// A whole address pasted into the host box ("http://nas:8080") fills all three boxes.
		if (/^[a-z]+:\/\/|:\d+(\/.*)?$/i.test(host.trim())) ({ ssl, host, port, basePath } = splitDlAddress(host, port));
		downloaderStep.dl.url = joinDlAddress(ssl, host, port, basePath);
		downloaderStep.dlError = '';
		downloaderStep.dlChecked = false;
	}
	// The network search knows these two (fixed default port, a page it can tell apart).
	const findable = $derived(downloaderStep.dl.kind === 'qbittorrent' || downloaderStep.dl.kind === 'sabnzbd');
	async function findHere() {
		const kind = downloaderStep.dl.kind;
		finding = true;
		findNote = '';
		try {
			const r = await api.post<{ found: { kind: string; address: string }[] }>('/migrate/find', { host: location.hostname, addresses: [downloaderStep.dl.url].filter(Boolean), kinds: [kind] });
			found = (r.found ?? []).filter((f) => f.kind === kind).map((f) => f.address);
			if (found.length) pickFound('0');
			else findNote = t('welcome.dl.notFound', { name: kindWords[kind] });
		} catch (err) {
			findNote = message(err);
		} finally {
			finding = false;
		}
	}
	function pickFound(v: string) {
		pick = v;
		if (v === 'manual') return;
		({ ssl, host, port, basePath } = splitDlAddress(found[Number(v)], downloaderStep.dlPorts[downloaderStep.dl.kind]));
		typed();
	}
	const keyPage = $derived(downloaderStep.dl.kind === 'sabnzbd' ? sabKeyPage(ssl, host, port, basePath) : '');
	function rowKeyPage(url: string): string {
		const s = splitDlAddress(url, '8080');
		return sabKeyPage(s.ssl, s.host, s.port, s.basePath);
	}
	// An imported SABnzbd being edited: the old app never shares its key, so it is pasted once here.
	const notBrought = $derived(downloaderStep.dl.kind === 'sabnzbd' && downloaderStep.dlEditing != null && downloaderStep.found[downloaderStep.dlEditing] ? appName(downloaderStep.found[downloaderStep.dlEditing]) : '');
	const secretHint = $derived(notBrought || (downloaderStep.dl.kind === 'qbittorrent' && downloaderStep.qbitWay === 'key') ? 'dl-secret-hint' : '');
</script>

<form class="dlform" onsubmit={downloaderStep.saveDownloader} data-testid="welcome-downloader">
	{#if downloaderStep.dlLoadError}
		<LoadError hasData={false} message={downloaderStep.dlLoadError} onretry={downloaderStep.loadDownloader} />
	{:else if downloaderStep.dlList.length}
		<ul class="conn" aria-label={t('welcome.connectedDownloaders')}>
			{#each downloaderStep.dlList as d (d.id)}
				{@const test = downloaderStep.dlTests[d.id]}
				<li>
					<div class="conn-row"><div class="conn-text"><b>{d.name}</b><span class="addr" dir="ltr">{d.url.replace(/^https?:\/\//, '')}</span>{#if downloaderStep.found[d.id]}<span class="found">{t('welcome.foundIn', { app: appName(downloaderStep.found[d.id]) })}</span>{/if}</div>
					{#if test}<TestTag ok={test.ok} text={test.text} app={kindWords[d.kind]} />{/if}</div>
					{#if test && !test.ok && downloaderStep.hidden[d.id]}<BroughtKey id={d.id} kind={d.kind} app={appName(downloaderStep.found[d.id])} needs={downloaderStep.hidden[d.id].needs} onsaved={(r) => downloaderStep.keySaved(d.id, r)} />
					{:else if test && !test.ok && downloaderStep.found[d.id] && d.kind === 'sabnzbd'}<p class="hint" data-testid="dl-row-not-brought">{t('welcome.dl.keyNotBrought', { app: appName(downloaderStep.found[d.id]) })}{#if rowKeyPage(d.url)} <a href={rowKeyPage(d.url)} target="_blank" rel="noopener noreferrer">{t('welcome.link.getKey')}</a>{/if}</p>
					{:else if test && !test.ok && downloaderStep.found[d.id]}<p class="hint">{t('welcome.cantReachYet')}</p>
					{:else if test && !test.ok}<p class="form-error" role="alert">{test.text}</p>{/if}
					{#if test?.ok && !downloaderStep.dlForm}<KindCategories id={d.id} name={d.name} main={d.category} movies={d.category_movies} shows={d.category_shows} music={d.category_music} save />{/if}
					{#if !downloaderStep.dlForm}
						<div class="row-acts">
							<button type="button" class="quiet" disabled={busy} onclick={() => downloaderStep.editDl(d)} aria-label={t('welcome.edit', { name: d.name })}>{t('welcome.edit2')}</button>
							<button type="button" class="quiet danger" disabled={busy} onclick={() => downloaderStep.removeDl(d)} aria-label={t('welcome.remove', { name: d.name })}>{t('welcome.remove2')}</button>
						</div>
					{/if}
				</li>
			{/each}
		</ul>
		{#if !downloaderStep.dlForm}
			<button class="add-row" type="button" disabled={busy} onclick={() => { error = ''; downloaderStep.dlAdding = true; }}><Icon name="plus" size={16} />{t('welcome.addAnother')}</button>
		{/if}
	{/if}
	{#if downloaderStep.dlForm}
		<div class="tiles seven" role="radiogroup" aria-label={t('welcome.downloaderApp')}>
			{#each downloaderStep.dlKinds as k (k)}
				<label class="tile small" class:on={downloaderStep.dl.kind === k}>
					<input type="radio" name="dlkind" value={k} checked={downloaderStep.dl.kind === k} onchange={() => downloaderStep.pickDl(k)} aria-label={kindWords[k]} />
					<span class="tile-label">{kindWords[k]}</span>
				</label>
			{/each}
		</div>
		<!-- The form on the left, what connecting does on the right (one column below 1100px). -->
		<div class="cols">
			<div class="dl-left">
				<div class="sfield">
					<div class="addr-labels"><label for="dl-scheme" class="proto-label">{t('welcome.server.protocol')}</label><label for={found.length && pick !== 'manual' ? 'dl-pick' : 'dl-url'}>{t('welcome.server.ipOrHostPlain')}</label><label for="dl-port" class="port-label">{t('welcome.server.port')}</label></div>
					<div class="addr-line">
						<button id="dl-scheme" type="button" class="input scheme" class:secure={ssl} onclick={() => { ssl = !ssl; typed(); }} aria-label={t('welcome.server.schemeFlip', { scheme: ssl ? 'https' : 'http' })} data-testid="dl-scheme">{ssl ? 'https' : 'http'}://</button>
						<div class="addr-row" class:is-select={found.length && pick !== 'manual'} class:no-find={!findable}>
							{#if found.length && pick !== 'manual'}
								<select id="dl-pick" class="input" value={pick} onchange={(e) => pickFound(e.currentTarget.value)} data-testid="dl-pick">
									<option value="manual">{t('welcome.server.manual')}</option>
									{#each found as f, i (f)}<option value={String(i)}>{f.replace(/^https?:\/\//, '').replace(/:\d+\/?$/, '')}</option>{/each}
								</select>
							{:else}
								<input id="dl-url" class="input" dir="ltr" type="text" inputmode="url" bind:value={host} oninput={typed} placeholder="192.168.1.10" autocomplete="off" spellcheck="false" aria-invalid={downloaderStep.dlError ? 'true' : undefined} aria-describedby={downloaderStep.dlError ? 'dl-url-error' : findNote ? 'dl-find-note' : undefined} />
							{/if}
							{#if findable}<span class="in-field">{#if found.length && pick !== 'manual'}<span class="chev" aria-hidden="true"><Icon name="chevronDown" size={16} /></span><span class="sep" aria-hidden="true"></span>{/if}<IconButton icon="refresh" variant="tinted" label={t('welcome.dl.find', { name: kindWords[downloaderStep.dl.kind] })} onclick={findHere} disabled={finding || busy} aria-busy={finding} data-testid="dl-find" /></span>{/if}
						</div>
						<input id="dl-port" class="input port" dir="ltr" inputmode="numeric" bind:value={port} oninput={typed} autocomplete="off" />
					</div>
					{#if downloaderStep.dlError}<p class="form-error" id="dl-url-error" role="alert">{downloaderStep.dlError}</p>{:else if findNote}<span class="hint" id="dl-find-note" role="status">{findNote}</span>{/if}
				</div>
				{#if downloaderStep.dl.kind === 'qbittorrent'}
					<fieldset class="way" data-testid="welcome-qbit-way">
						<legend class="label">{t('settings.downloaders.signInWith')}</legend>
						<div class="seg">
							<label class:on={downloaderStep.qbitWay === 'key'}><input type="radio" name="qbit-way" value="key" checked={downloaderStep.qbitWay === 'key'} onchange={() => downloaderStep.pickWay('key')} />{t('welcome.apiKeyLabel')}</label>
							<label class:on={downloaderStep.qbitWay === 'login'}><input type="radio" name="qbit-way" value="login" checked={downloaderStep.qbitWay === 'login'} onchange={() => downloaderStep.pickWay('login')} />{t('welcome.passwordLabel')}</label>
						</div>
					</fieldset>
				{/if}
				<div class="two">
					{#if downloaderStep.dlUser}
						<div class="field">
							<label for="dl-user">{t('welcome.account.username')}</label>
							<input id="dl-user" class="input" bind:value={downloaderStep.dl.username} oninput={() => { downloaderStep.dlAuth = ''; downloaderStep.dlChecked = false; }} autocomplete="off" autocapitalize="none" spellcheck="false" aria-invalid={downloaderStep.dlAuth ? 'true' : undefined} aria-describedby={downloaderStep.dlAuth ? 'dl-user-error' : undefined} />
							{#if downloaderStep.dlAuth}<p class="form-error" id="dl-user-error" role="alert">{downloaderStep.dlAuth}</p>{/if}
						</div>
					{/if}
					<div class="field">
						<label for="dl-secret">{downloaderStep.dl.kind === 'sabnzbd' || (downloaderStep.dl.kind === 'qbittorrent' && downloaderStep.qbitWay === 'key') ? t('welcome.apiKeyLabel') : downloaderStep.dl.kind === 'aria2' ? t('welcome.secretTokenLabel') : t('welcome.passwordLabel')}</label>
						<div class="key-row">
							<input id="dl-secret" class="input" class:has-get={!!keyPage} type="password" bind:value={downloaderStep.dl.secret} oninput={() => (downloaderStep.dlChecked = false)} autocomplete="new-password" spellcheck="false" aria-describedby={secretHint || undefined} />
							{#if keyPage}<span class="key-acts"><a class="key-get" href={keyPage} target="_blank" rel="noopener noreferrer" data-testid="dl-key-page">{t('welcome.link.getKey')}<Icon name="arrowUpRight" size={14} /></a></span>{/if}
						</div>
						{#if notBrought}<span class="hint" id="dl-secret-hint" data-testid="dl-key-not-brought">{t('welcome.dl.keyNotBrought', { app: notBrought })}</span>
						{:else if downloaderStep.dl.kind === 'qbittorrent' && downloaderStep.qbitWay === 'key'}<span class="hint" id="dl-secret-hint">{t('settings.downloaders.secretQbit')}</span>{/if}
					</div>
				</div>
				{#if downloaderStep.dlChecked}<p class="hint ok-text" role="status" data-testid="dl-checked">{t('welcome.dl.checked', { name: kindWords[downloaderStep.dl.kind], action: t('welcome.connect', { name: kindWords[downloaderStep.dl.kind] }) })}</p>{/if}
				<div class="sect-acts">
					{#if downloaderStep.dlEditing == null}<Button type="button" disabled={busy || !host.trim()} busy={downloaderStep.dlChecking} onclick={downloaderStep.checkDl} data-testid="dl-check">{t('welcome.server.check')}</Button>{/if}
					<Button type="submit" variant="primary" disabled={busy || downloaderStep.dlChecking} {busy}>{downloaderStep.dlEditing != null ? t('welcome.save') : t('welcome.connect', { name: kindWords[downloaderStep.dl.kind] })}</Button>
					{#if downloaderStep.dlList.length}<button class="alt" type="button" disabled={busy} onclick={() => { error = ''; downloaderStep.resetForm(); }}>{t('welcome.cancel')}</button>{/if}
				</div>
			</div>
			<aside class="brings" aria-labelledby="dl-brings" data-testid="dl-brings">
				<h2 id="dl-brings">{t('welcome.dl.brings.title')}</h2>
				<ul class="btiles">{#each DL_BRINGS as l (l)}<li class="btile"><span class="bname">{t(l, { name: kindWords[downloaderStep.dl.kind] })}</span><Icon name="check" size={14} /></li>{/each}</ul>
				<p class="hint">{t('welcome.dl.brings.stays', { name: kindWords[downloaderStep.dl.kind] })}</p>
			</aside>
		</div>
	{/if}
</form>

<style>
	.cols { display: flex; flex-direction: column; gap: var(--s-5, 24px); }
	.dl-left { min-width: 0; }
	@media (max-width: 1099px) { .cols .brings { display: none; } }
	@media (min-width: 1100px) { .cols { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); column-gap: 40px; align-items: start; } }
	.sfield { min-width: 0; display: grid; gap: 6px; text-align: center; }
	.sfield .input { text-align: start; width: 100%; box-sizing: border-box; }
	.sfield > .hint, .brings .hint { color: var(--text-muted); font: var(--t-footnote); }
	.ok-text { color: var(--ok-text); }
	.addr-labels { position: relative; }
	.addr-labels > label { display: block; }
	.addr-labels .proto-label, .addr-labels .port-label { position: absolute; top: 0; width: 80px; text-align: center; }
	.addr-labels .proto-label { inset-inline-start: 0; }
	.addr-labels .port-label { inset-inline-end: 0; }
	.addr-line { display: grid; grid-template-columns: 80px minmax(0, 1fr) 80px; gap: 14px; }
	.addr-row { position: relative; min-width: 0; }
	.addr-row .input { width: 100%; padding-inline-end: 52px; }
	.addr-row.no-find .input { padding-inline-end: 12px; }
	.addr-row.is-select .input { appearance: none; -webkit-appearance: none; padding-inline-end: 92px; cursor: pointer; }
	.addr-line .scheme { display: flex; align-items: center; justify-content: center; width: 80px; min-width: 0; padding: 0; font-weight: 400; cursor: pointer; }
	.addr-line .scheme.secure { border-color: rgb(52 199 89 / 0.5); }
	.addr-line .port { text-align: center; padding: 0 8px; }
	.in-field { position: absolute; inset-inline-end: 6px; top: 50%; transform: translateY(-50%); display: flex; align-items: center; gap: 2px; pointer-events: none; }
	.in-field :global(button) { pointer-events: auto; }
	.chev { display: flex; width: 28px; justify-content: center; opacity: 0.75; }
	.sep { width: 1px; height: 20px; background: currentColor; opacity: 0.2; margin-inline: 2px; }
	.key-row { position: relative; }
	.key-row .input.has-get { padding-inline-end: 110px; }
	.key-acts { position: absolute; inset-inline-end: 6px; top: 6px; bottom: 6px; display: flex; gap: 6px; }
	.key-get { display: inline-flex; align-items: center; gap: 4px; padding: 0 12px; border-radius: 10px; background: var(--accent); color: var(--on-accent, #fff); font: 600 13px/1 var(--font-ui); text-decoration: none; white-space: nowrap; }
	.key-get:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
	@media (hover: hover) { .key-get:hover { filter: brightness(1.08); } }
	.brings h2 { margin: 0 0 10px; font: 600 15px/20px var(--font-ui); text-align: start; }
	.brings .hint { margin: 10px 0 0; text-align: start; }
	.btiles { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 6px; text-align: start; }
	.btile { display: flex; flex-direction: column; gap: 2px; padding: 7px 10px; min-height: 48px; box-sizing: border-box; border: 1px solid var(--g-line, rgb(255 255 255 / 0.12)); border-radius: 10px; background: var(--g-bg, rgb(255 255 255 / 0.05)); }
	.btile :global(svg) { color: var(--ok-text); }
	.bname { font-size: 12px; opacity: 0.7; }
	@media (max-width: 600px) {
		.addr-line { grid-template-columns: 64px minmax(0, 1fr) 64px; gap: 8px; }
		.addr-line .scheme { width: 64px; }
		.addr-labels .proto-label, .addr-labels .port-label { width: 64px; }
		.key-acts { position: static; margin-top: 6px; justify-content: center; }
		.key-acts > * { min-height: 44px; }
		.key-row .input.has-get { padding-inline-end: 12px; }
	}
	.sect-acts { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; margin-top: var(--s-4); }
	/* Cancel: the quiet link look of the wizard's other secondary choices. */
	.alt { min-height: 44px; padding: 0 12px; border: 0; border-radius: var(--r-sm); background: none; color: var(--text-muted); font: 500 15px/1 var(--font-ui); cursor: pointer; }
	@media (hover: hover) { .alt:hover:not(:disabled) { color: var(--text); background: color-mix(in srgb, var(--text) 6%, transparent); } }
	.alt:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.alt:disabled { opacity: 0.5; cursor: default; }
	:global(.glass) .alt { color: rgb(var(--ink-rgb) / 0.7); }
	fieldset { border: 0; padding: 0; margin: 0; min-width: 0; }
	legend { margin-bottom: 12px; font: var(--t-headline); color: var(--text); }
	/* Tiles: an icon and a word; picked tiles take the accent, with a tick. */
	.tiles { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px; }
	.tiles.seven { grid-template-columns: repeat(4, minmax(0, 1fr)); margin-bottom: var(--s-4); }
	.tile {
		position: relative;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 10px;
		min-height: 108px;
		padding: 14px 8px;
		border-radius: 16px;
		border: 1px solid var(--line);
		background: var(--surface-2);
		color: var(--text-muted);
		font: inherit;
		text-decoration: none;
		cursor: pointer;
		transition: border-color var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out);
	}
	.tile.small { min-height: 56px; padding: 10px 6px; }
	/* The real control covers the whole tile, invisible, so taps and clicks land on it. */
	.tile input { position: absolute; inset: 0; width: 100%; height: 100%; margin: 0; opacity: 0; cursor: pointer; }
	.tile-label { font: 600 14px/1.2 var(--font-ui); color: var(--text); text-align: center; }
	@media (hover: hover) { .tile:hover { border-color: var(--line-strong); } }
	.tile:active { transform: scale(0.98); }
	.tile.on { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 7%, var(--surface-1)); color: var(--accent-text); }
	.tile:has(input:focus-visible) { box-shadow: var(--focus-ring); }
	/* Family: a two-way switch, the picked side lifted. */
	.way { margin-top: var(--s-4); }
	.way legend { margin-bottom: 8px; }
	.seg { display: grid; grid-template-columns: 1fr 1fr; gap: 4px; padding: 4px; border-radius: 12px; background: var(--surface-2); border: 1px solid var(--line); }
	.seg label {
		position: relative;
		display: grid;
		place-items: center;
		min-height: 40px;
		padding: 6px 10px;
		border-radius: 9px;
		color: var(--text-muted);
		font: 500 15px/1.2 var(--font-ui);
		text-align: center;
		overflow-wrap: anywhere;
		cursor: pointer;
		transition: background-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out);
	}
	.seg input { position: absolute; inset: 0; width: 100%; height: 100%; margin: 0; opacity: 0; cursor: pointer; }
	.seg label.on { background: var(--surface-1); color: var(--text); box-shadow: 0 1px 3px rgb(0 0 0 / 0.14), 0 0 0 0.5px var(--line); }
	.seg label:has(input:focus-visible) { box-shadow: var(--focus-ring); }
	.hint { display: block; margin-top: var(--s-2); color: var(--text-muted); font: var(--t-footnote); }
	/* A connected downloader or source: a plain row, its version as a small tag. */
	.conn { list-style: none; margin: 0 0 12px; padding: 0; display: grid; gap: 10px; }
	.conn-row { display: flex; align-items: center; gap: 12px; padding: 14px 16px; border-radius: 16px; background: var(--surface-2); border: 1px solid var(--line); }
	.conn-text { flex: 1; min-width: 0; }
	.conn-text b { display: block; font: 600 15px/1.3 var(--font-ui); color: var(--text); }
	.conn-text .addr, .conn-text .found { display: block; margin-top: 2px; font-size: 12.5px; color: var(--text-muted); overflow-wrap: anywhere; }
	.quiet { flex: none; align-self: center; display: inline-flex; align-items: center; min-height: 32px; border: 0; padding: 0 8px; background: none; color: var(--text-muted); font: 500 13px/1.2 var(--font-ui); cursor: pointer; }
	.quiet:hover:not(:disabled) { color: var(--text); }
	.quiet.danger:hover:not(:disabled) { color: var(--danger-text, #ff6961); }
	.row-acts { display: flex; gap: 16px; padding: 6px 4px 0; }
	.quiet:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 6px; }
	/* Add another: the same pill as Manage and Add another on every setup step. */
	.add-row { align-self: flex-start; display: inline-flex; align-items: center; gap: 6px; min-height: 40px; height: 40px; padding: 0 16px; border: 1px solid var(--g-line-hover, var(--line-strong)); border-radius: 999px; background: none; color: var(--ink, var(--text)); font: 600 14px/1 var(--font-ui); white-space: nowrap; cursor: pointer; }
	.add-row:hover:not(:disabled) { background: rgb(var(--ink-rgb, 255 255 255) / 0.12); }
	.add-row:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.two { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: var(--s-4); margin-top: var(--s-4); align-items: start; }
	.two > :global(.field) { margin: 0; }
	@media (max-width: 520px) {
		.tiles { grid-template-columns: repeat(2, minmax(0, 1fr)); }
		.tiles.seven { grid-template-columns: repeat(2, minmax(0, 1fr)); }
		.tile { min-height: 96px; }
		.tile.small { min-height: 52px; }
		/* Downloads on a phone: the seven apps in two short rows, user and password side by side, so Continue stays on screen. */
		.tiles.seven { grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 6px; margin-bottom: var(--s-3); }
		.tile.small { min-height: 44px; padding: 4px 2px; border-radius: 12px; }
		.tiles.seven .tile-label { font-size: 12px; overflow-wrap: anywhere; }
		.two { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 10px; margin-top: var(--s-3); }
		.way { margin-top: var(--s-3); }
		.sect-acts { margin-top: var(--s-3); }
		/* One field (no user name): it takes the full width. */
		.two:has(> .field:only-child) { grid-template-columns: minmax(0, 1fr); }
	}
	@media (pointer: coarse), (max-width: 520px) {
		.seg label { min-height: 44px; }
		.quiet { min-height: 44px; padding: 0 12px; }
	}
	/* One layer inside the glass card: lists are flat rows split by hairlines, choices are outlined, and only text fields keep a filled box. */
	.tile { background: transparent; }
	@media (hover: hover) { .tile:hover { background: color-mix(in srgb, var(--text) 4%, transparent); } }
	.conn { gap: 0; border-top: 1px solid var(--line); }
	.conn > li { border-bottom: 1px solid var(--line); }
	.conn-row { background: none; border: 0; border-radius: 0; padding: 14px 2px 4px; }
	.conn .row-acts { padding: 0 0 12px; }
	:global(.glass) .field > label, :global(.glass) .way legend { font: 600 13px/1.3 var(--font-ui); color: rgb(var(--ink-rgb) / 0.85); }
	:global(.glass) .way legend { width: 100%; text-align: center; }
	:global(.glass) .hint { color: rgb(var(--ink-rgb) / 0.55); }
	:global(.glass) .conn { border-color: rgb(var(--ink-rgb) / 0.1); }
	:global(.glass) .conn > li { border-color: rgb(var(--ink-rgb) / 0.1); }
	:global(.glass) .conn-text b { color: var(--ink); }
	:global(.glass) .conn-text .addr, :global(.glass) .conn-text .found { color: rgb(var(--ink-rgb) / 0.6); }
	:global(.glass) .add-row { border-color: rgb(var(--ink-rgb) / 0.18); color: rgb(var(--ink-rgb) / 0.7); }
	/* Choice tiles: outlined, see-through, a faint fill on hover. */
	:global(.glass) .tile { border-color: rgb(var(--ink-rgb) / 0.14); background: transparent; color: rgb(var(--ink-rgb) / 0.75); }
	@media (hover: hover) { :global(.glass) .tile:hover { border-color: rgb(var(--ink-rgb) / 0.24); background: rgb(var(--ink-rgb) / 0.05); } }
	:global(.glass) .tile-label { color: var(--ink); }
	:global(.glass) .tile.on { border-color: rgb(var(--ink-rgb) / 0.9); background: rgb(var(--ink-rgb) / 0.1); color: var(--ink); }
	:global(.glass) .conn { background: var(--g-bg); border: 1px solid var(--g-line); border-radius: var(--g-r); overflow: hidden; }
	:global(.glass) .conn-row, :global(.glass) .conn .row-acts { padding-left: 14px; padding-right: 14px; }
	:global(.glass) .conn > li:last-child { border-bottom: 0; }
	:global(.glass) .tile, :global(.glass) .add-row { border-radius: var(--g-r); }
	:global(.glass) .tile { border-color: var(--g-line); background: var(--g-bg); }
	@media (hover: hover) { :global(.glass) .tile:hover { border-color: var(--g-line-hover); background: var(--g-bg-hover); } }
	:global(.glass) .seg { border-radius: var(--g-r); background: var(--g-bg); border-color: var(--g-line); }
	:global(.glass) .seg label { color: rgb(var(--ink-rgb) / 0.7); border-radius: calc(var(--g-r) - 3px); }
	:global(.glass) .seg label.on { background: rgb(var(--ink-rgb) / 0.92); color: var(--ink-inv); box-shadow: none; }
	@media (min-width: 1100px) {
		:global(.glass) .tile { min-height: clamp(120px, 22vh, 260px); gap: 14px; }
		:global(.glass) .tile-label { font-size: 20px; }
		:global(.glass) .tile.small { min-height: clamp(64px, 8vh, 96px); }
		:global(.glass) .tiles.seven { margin-bottom: var(--s-2); gap: 8px; }
		.tiles.seven { grid-template-columns: repeat(7, minmax(0, 1fr)); }
		:global(.glass) .tiles.seven .tile-label { font-size: 16px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 100%; }
		/* Downloads: the apps in one row, then the address beside the sign-in fields. */
		.dlform { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 16px 24px; align-items: start; }
		.dlform > * { grid-column: 1 / -1; }
		/* The connected list stays first; the add form's tiles and sign-in switch follow it. */
		.dlform > .conn, .dlform > .add-row { order: -3; }
		.dlform > .tiles { order: -2; margin-bottom: 0; }
	}
</style>
