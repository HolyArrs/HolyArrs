<script lang="ts">
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import Confirm from '#lib/ui/Confirm.svelte';
	import GhostEmpty from './GhostEmpty.svelte';
	import { size as bytes, protocolOf } from './size.ts';
	import { fileTree } from './filetree.ts';
	import { filterFiles, indexesUnder } from './manage.ts';
	import { badKind, logDays, quietPeers } from './panel-words.ts';
	import { problem } from './problem.ts';
	import { stageOf } from './status-tabs.ts';
	import { repairSteps, serverRows } from './usenet.ts';
	import { live, type PaneDetail } from './pane-detail.svelte.ts';
	import { calmErr, PRIOS, TRACKER_WORD, fileIndexes, flagsOf, infoGroups, mmss, usenetGroups, type PaneTab } from './pane-facts.ts';
	import type { ImportRecord } from './types.ts';
	import { to } from '../core/paths.ts';
	import type { DownloadRow } from '../core/types.ts';

	let { r, tab, pane, importOf, watch, mayEdit }: { r: DownloadRow; tab: PaneTab; pane: PaneDetail; importOf: Map<number, ImportRecord>; watch: boolean; mayEdit: (r: DownloadRow) => boolean } = $props();

	const tor = $derived(protocolOf(r) === 'torrent');
	const d = $derived(pane.det.id === r.id ? pane.det.detail : undefined);
	const tt = $derived(d?.torrent);

	function trNav(e: KeyboardEvent) {
		if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
		const tr = e.currentTarget as HTMLElement;
		const next = (e.key === 'ArrowDown' ? tr.nextElementSibling : tr.previousElementSibling) as HTMLElement | null;
		if (!next) return;
		e.preventDefault();
		e.stopPropagation();
		next.focus();
		next.click();
	}
</script>

{#snippet calm(r: DownloadRow, tab: string, err: string | undefined)}
	{#if err}
		<p class="calm" role="status" data-testid="dl-panel-error"><span>{err}</span><button type="button" class="xbn" onclick={() => void pane.readDetail(r, tab)} data-testid="dl-panel-retry">{t('health.sections.tryAgain')}</button></p>
	{:else}
		<p class="calm wait" aria-live="polite" data-testid="dl-panel-wait">{t('downloads.tv.reading')}</p>
	{/if}
{/snippet}

	{#if tab === 'Info'}
		{@const shown = tor ? infoGroups(r, d) : usenetGroups(r, d, importOf.get(r.id))}
		<div class="facts" data-testid="dl-info">
			{#if stageOf(r) === 'failed' || problem(r)?.tone === 'fail'}<a class="hlink" href={to('/downloads/history?step=failed')} data-testid="dl-see-history">{t('downloads.tv.seeHistory')}</a>{/if}
			{#if tor && d?.torrent_error}<p class="calm slim" role="status" data-testid="dl-panel-error"><span>{calmErr()}</span><button type="button" class="xbn" onclick={() => void pane.readDetail(r, 'Info')} data-testid="dl-panel-retry">{t('health.sections.tryAgain')}</button></p>{/if}
			{#if shown.length}
				<div class="fcols" style="--fn: {shown.length}">
					{#each shown as g (g.cap)}
						<div class="fcol">
							<h3 class="fcap">{g.cap}</h3>
							<dl class="fgrid" aria-label={g.cap}>
								{#each g.rows as x (x.k)}<div class="fr"><dt>{x.k}</dt><dd class:path={x.path} title={x.path || x.v.length > 24 ? x.v : undefined} data-testid={x.tid}>{x.v}</dd></div>{/each}
							</dl>
						</div>
					{/each}
				</div>
			{:else}
				<p class="fnone">{t('downloads.tv.nothing')}</p>
			{/if}
		</div>
	{:else if tab === 'Files'}
		{@const files = pane.det.id === r.id ? pane.det.files : undefined}
		{#if !tor || !files}
			{@render calm(r, 'Files', tor && pane.det.id === r.id ? pane.det.filesErr : undefined)}
		{:else if !files.length}
			<GhostEmpty compact testid="dl-files-none" title={t('downloads.tv.noFiles')} text={t('downloads.tv.noFilesText')} />
		{:else}
			{@const shownF = filterFiles(files, pane.fq)}
			{@const rowsT = fileTree(shownF.map((f) => ({ name: f.name, size: f.size, done: f.size * f.progress })), (k) => !!pane.closed[`${r.id}|${k}`])}
			{@const keys = rowsT.map((x) => x.key)}
			{#if pane.prioWait && pane.prioWait.r.id === r.id && !watch}
				{@const w = pane.prioWait}
				<div class="calm prf" data-testid="dl-files-proof">
					<p class="xt">{t('downloads.tv.confirmPw')}</p>
					<Confirm action="password" onCancel={() => (pane.prioWait = null)}>
						<button type="button" class="xbn" disabled={pane.filesBusy} onclick={() => pane.setPrio(w.r, w.indexes, w.priority)} data-testid="dl-files-proof-go">{t('downloads.tv.changePrio')}</button>
					</Confirm>
				</div>
			{/if}
			<div class="fbar" hidden={files.length === 1}>
				<input type="search" class="tfld" bind:this={pane.fqEl} bind:value={pane.fq} placeholder={t('downloads.tv.filter')} aria-label={t('downloads.tv.filter')} onkeydown={(e) => { if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); pane.fq = ''; } }} data-testid="dl-files-filter" />
				{#if pane.fsel.size && !watch && mayEdit(r)}
					<span class="k" aria-live="polite" data-testid="dl-files-picked">{pane.fsel.size} {t('downloads.sel.selected')}</span>
					{#each PRIOS as o (o.v)}<button type="button" class="xbn" disabled={pane.filesBusy} onclick={() => pane.setPrio(r, indexesUnder(files, pane.fsel), o.v)} data-testid="dl-files-prio-{o.v}">{o.label}</button>{/each}
					<button type="button" class="xbn" onclick={() => (pane.fsel = new Set())} data-testid="dl-files-clear">{t('downloads.sel.clear')}</button>
				{/if}
			</div>
			{#if files.length === 1}
				{@const f = files[0]}
				{@const pc = Math.floor(f.progress * 100)}
				<div class="tblw" data-testid="dl-filetable"><table class="t2">
					<thead><tr><th class="c-name">{t('downloads.fact.name')}</th><th class="c-num c-size">{t('downloads.pf.size')}</th><th class="c-prog">{t('downloads.col.progress')}</th><th class="c-prio">{t('downloads.act.priority')}</th></tr></thead>
					<tbody><tr class:off={f.priority === 0} data-testid="dl-filerow" data-single="true">
						<td class="c-name"><span class="fn" title={f.name}>{f.name.split('/').pop()}</span></td>
						<td class="c-num c-size">{bytes(f.size)}</td>
						<td class="c-prog"><span class="pbar"><i style="width: {pc}%"></i></span><span class="pv">{pc}%</span></td>
						<td class="c-prio"><select class="pick" aria-label={t('downloads.tv.prioOf', { name: f.name })} value={PRIOS.some((o) => o.v === f.priority) ? f.priority : 1} disabled={watch || pane.filesBusy || !mayEdit(r)} onchange={(e) => pane.setPrio(r, [f.index], Number(e.currentTarget.value))}>{#each PRIOS as o (o.v)}<option value={o.v}>{o.label}</option>{/each}</select></td>
					</tr></tbody>
				</table></div>
			{:else if !shownF.length}
				<GhostEmpty compact testid="dl-files-nomatch" title={t('downloads.tv.noMatch')} text={t('downloads.tv.noMatchText')} />
			{:else}
			<div class="tblw" data-testid="dl-filetable"><table class="t2">
				<thead><tr><th class="c-chk"><span class="sr">{t('downloads.tv.download')}</span></th><th class="c-name">{t('downloads.fact.name')}</th><th class="c-num c-size">{t('downloads.pf.size')}</th><th class="c-prog">{t('downloads.col.progress')}</th><th class="c-prio">{t('downloads.act.priority')}</th><th class="c-num c-rem">{t('downloads.tv.remaining')}</th></tr></thead>
				<tbody>
					{#each rowsT as x, ri (x.key)}
						{@const pc = x.size ? Math.floor((x.done / x.size) * 100) : 0}
						{@const f = x.dir ? undefined : shownF[x.i]}
						{@const kids = x.dir ? files.filter((y) => y.name.startsWith(x.key + '/')) : []}
						{@const skipped = f ? f.priority === 0 : kids.length > 0 && kids.every((y) => y.priority === 0)}
						<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
						<tr class:sel={pane.sel[`${r.id}:f`] === x.key || pane.fsel.has(x.key)} class:off={skipped} tabindex={ri === 0 ? 0 : -1} aria-selected={pane.fsel.has(x.key)} onclick={(e) => pane.fileClick(e, r, keys, x.key)} onkeydown={(e) => { trNav(e); if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); pane.fileClick(e, r, keys, x.key); } }} data-testid="dl-filerow">
							<td class="c-chk"><input type="checkbox" checked={!skipped} disabled={watch || pane.filesBusy || !mayEdit(r)} aria-label={t('downloads.tv.downloadFile', { name: x.name })} onclick={(e) => e.stopPropagation()} onchange={(e) => pane.setPrio(r, fileIndexes(files, x.key), e.currentTarget.checked ? 1 : 0)} /></td>
							<td class="c-name"><span class="fnm" style="padding-left: {x.depth * 14}px">{#if x.dir}<button type="button" class="fold" class:shut={pane.closed[`${r.id}|${x.key}`]} aria-label={t(pane.closed[`${r.id}|${x.key}`] ? 'downloads.tv.expandName' : 'downloads.tv.collapseName', { name: x.name })} aria-expanded={!pane.closed[`${r.id}|${x.key}`]} onclick={(e) => { e.stopPropagation(); pane.closed[`${r.id}|${x.key}`] = !pane.closed[`${r.id}|${x.key}`]; }}><Icon name="chevronDown" size={14} stroke={2.2} /></button>{:else}<span class="fold sp0"></span>{/if}<span class="fn" title={x.key}>{x.name}</span></span></td>
							<td class="c-num c-size">{bytes(x.size)}</td>
							<td class="c-prog"><span class="pbar"><i style="width: {pc}%"></i></span><span class="pv">{pc}%</span></td>
							<td class="c-prio">{#if f}<select class="pick" aria-label={t('downloads.tv.prioOf', { name: x.name })} value={PRIOS.some((o) => o.v === f.priority) ? f.priority : 1} disabled={watch || pane.filesBusy || !mayEdit(r)} onclick={(e) => e.stopPropagation()} onchange={(e) => pane.setPrio(r, [f.index], Number(e.currentTarget.value))}>{#each PRIOS as o (o.v)}<option value={o.v}>{o.label}</option>{/each}</select>{/if}</td>
							<td class="c-num c-rem">{bytes(Math.max(0, x.size - x.done))}</td>
						</tr>
					{/each}
				</tbody>
			</table></div>
			{/if}
		{/if}
	{:else if tab === 'Peers'}
		{@const peers = pane.det.id === r.id ? pane.det.peers : undefined}
		{#if !peers}
			{@render calm(r, 'Peers', pane.det.id === r.id ? pane.det.swarmErr : undefined)}
		{:else if !peers.length && r.progress >= 1}
			<p class="quiet0" role="status" data-testid="dl-peers-quiet">{quietPeers(tt?.last_activity)}</p>
		{:else}
			<div class="tblw" data-testid="dl-peertable"><table class="t2">
				<thead><tr><th class="c-name">{t('downloads.tv.address')}</th><th class="c-cl">{t('downloads.tv.client')}</th><th class="c-prog">{t('downloads.col.progress')}</th><th class="c-num">↓</th><th class="c-num">↑</th><th class="c-fl">{t('downloads.tv.flags')}</th></tr></thead>
				<tbody>
					{#each peers as p, ri (p.address)}
						<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
						<tr class:sel={pane.sel[`${r.id}:p`] === p.address} tabindex={ri === 0 ? 0 : -1} onclick={() => (pane.sel[`${r.id}:p`] = p.address)} onkeydown={(e) => { trNav(e); if (e.key === 'Enter') pane.sel[`${r.id}:p`] = p.address; }} data-testid="dl-peerrow">
							<td class="c-name"><span class="fn">{p.address}</span></td>
							<td class="c-cl"><span class="fn">{p.client || '–'}</span></td>
							<td class="c-prog"><span class="pbar"><i style="width: {Math.floor(p.progress * 100)}%"></i></span><span class="pv">{Math.floor(p.progress * 100)}%</span></td>
							<td class="c-num">{bytes(p.speed)}/s</td>
							<td class="c-num">{bytes(p.up_speed)}/s</td>
							<td class="c-fl">{flagsOf(p)}</td>
						</tr>
					{/each}
					{#if !peers.length}<tr><td colspan="6"><GhostEmpty compact testid="dl-peers-none" title={t('downloads.tv.noPeers')} text={r.progress >= 1 ? t('downloads.tv.nobody') : t('downloads.tv.peersShow')} /></td></tr>{/if}
				</tbody>
			</table></div>
		{/if}
	{:else if tab === 'Trackers'}
		{@const trs = pane.det.id === r.id ? pane.det.trackers : undefined}
		{#if !trs}
			{@render calm(r, 'Trackers', pane.det.id === r.id ? pane.det.swarmErr : undefined)}
		{:else}
			<div class="tblw" data-testid="dl-trackertable"><table class="t2">
				<thead><tr><th class="c-name">{t('downloads.col.tracker')}</th><th class="c-num c-tier">{t('downloads.tv.tier')}</th><th>{t('downloads.col.status')}</th><th class="c-num">{t('downloads.facet.peers')}</th><th class="c-num">{t('downloads.facet.seeds')}</th><th class="c-msg">{t('downloads.hd.message')}</th><th class="c-num">{t('downloads.tv.nextCheck')}</th></tr></thead>
				<tbody>
					{#each trs as x, ri (x.url + x.tier)}
						<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
						<tr class:sel={pane.sel[`${r.id}:t`] === (x.ref || x.url)} class:fixed={x.fixed} tabindex={ri === 0 ? 0 : -1} onclick={() => pane.pickTracker(r, x)} onkeydown={(e) => { trNav(e); if (e.key === 'Enter') pane.pickTracker(r, x); }} data-testid="dl-trackerrow" data-fixed={x.fixed ? 'true' : undefined}>
							<td class="c-name"><span class="fn">{x.url}</span></td>
							<td class="c-num c-tier">{x.fixed ? '' : x.tier}</td>
							<td><span class="st-{x.status === 'working' ? 'working' : x.status === 'not_contacted' ? 'updating' : 'error'}">{TRACKER_WORD[x.status] ?? x.status}</span></td>
							<td class="c-num">{x.peers}</td>
							<td class="c-num">{x.seeds}</td>
							<td class="c-msg"><span class="fn">{x.message}</span></td>
							<td class="c-num">{x.next_check ? mmss(x.next_check) : x.fixed ? '' : '–'}</td>
						</tr>
					{/each}
					{#if !trs.length}<tr><td colspan="7"><GhostEmpty compact testid="dl-trackers-none" title={t('downloads.tv.noTrackers')} text={t('downloads.tv.noTrackersText')} /></td></tr>{/if}
				</tbody>
			</table></div>
			{#if !watch && mayEdit(r) && r.downloader_id && r.hash}
				{@const chosen = trs.find((x) => x.ref && pane.sel[`${r.id}:t`] === x.ref)}
				<div class="tred" data-testid="dl-tracker-edit">
					{#if chosen}
						<p class="tl"><b>{chosen.url}</b> <span class="k">{t('downloads.tv.hidden')}</span></p>
						<div class="trow">
							<input type="text" class="tfld" placeholder={t('downloads.tv.newAddress')} bind:value={pane.trUrl} spellcheck="false" aria-label={t('downloads.tv.newTracker')} data-testid="dl-tracker-url" />
							<input type="number" class="tfld tier" min="0" max="255" bind:value={pane.trTier} aria-label={t('downloads.tv.tier')} data-testid="dl-tracker-tier" />
							<button type="button" class="xbn" disabled={pane.trBusy} onclick={() => pane.saveTracker(r, chosen)} data-testid="dl-tracker-save">{t('downloads.tv.save')}</button>
							<button type="button" class="xbn" disabled={pane.trBusy} onclick={() => pane.removeTracker(r, chosen)} data-testid="dl-tracker-remove">{t('downloads.act.remove')}</button>
						</div>
					{/if}
					<div class="trow">
						<textarea class="tfld" rows="1" placeholder={t('downloads.tv.addTrackers')} bind:value={pane.trAdd} spellcheck="false" aria-label={t('downloads.tv.addTrackersLabel')} data-testid="dl-tracker-add-input"></textarea>
						<button type="button" class="xbn" disabled={pane.trBusy || !pane.trAdd.trim()} onclick={() => pane.addTrackers(r)} data-testid="dl-tracker-add">{t('downloads.at.add')}</button>
					</div>
				</div>
			{/if}
		{/if}
	{:else if tab === 'Servers'}
		{@const uv = pane.det.id === r.id ? pane.det.usenet : undefined}
		{@const srv = serverRows(uv)}
		{#if !uv && live(r)}
			{@render calm(r, 'Servers', pane.det.id === r.id ? pane.det.usenetErr : undefined)}
		{:else if srv.length}
			<div class="tblw" data-testid="dl-servertable"><table class="t2">
				<thead><tr><th class="c-name">{t('downloads.tv.server')}</th><th class="c-num">{t('downloads.tv.thisDownload')}</th><th class="c-num">{t('downloads.tv.found')}</th></tr></thead>
				<tbody>
					{#each srv as s (s.name)}
						<tr data-testid="dl-serverrow"><td class="c-name"><span class="fn">{s.name}</span></td><td class="c-num">{s.job || '–'}</td><td class="c-num">{s.found || '–'}</td></tr>
					{/each}
				</tbody>
			</table></div>
		{:else}
			<GhostEmpty compact testid="dl-servers-none" title={t('downloads.tv.noServers')} text={t('downloads.tv.noServersText')} />
		{/if}
	{:else}
		{@const log = d?.log ?? []}
		<!-- A Usenet job's verify, repair and unpack steps are its newest lines, so they lead the Log. -->
		{@const steps = tor ? [] : repairSteps(pane.det.id === r.id ? pane.det.usenet : undefined)}
		{#if log.length || steps.length}
			<div class="lgw" data-testid="dl-log">
				{#each steps as g (g.title)}
					<h4 class="lgday">{g.title}</h4>
					<ol class="lg" data-testid="dl-repair">
						{#each g.lines as l, i (i)}<li class:bad={badKind(l)}><time></time><span class="lgd" aria-hidden="true"></span><span class="lgt">{l}</span></li>{/each}
					</ol>
				{/each}
				{#each logDays(log) as g (g.day)}
					<h4 class="lgday">{g.day}</h4>
					<ol class="lg">
						{#each g.lines as l, i (i)}<li class:bad={badKind(l.kind)}><time>{new Date(l.at).toLocaleTimeString(appLocale(), { hour: '2-digit', minute: '2-digit' })}</time><span class="lgd" aria-hidden="true"></span><span class="lgt">{serverWords(l.summary)}</span></li>{/each}
					</ol>
				{/each}
			</div>
		{:else if !d}
			{@render calm(r, 'Log', undefined)}
		{:else}
			<GhostEmpty compact testid="dl-log-none" title={t('downloads.tv.nothingLogged')} text={t('downloads.tv.nothingLoggedText')} />
		{/if}
	{/if}

<style>
	.k { color: var(--text-muted); font: var(--t-footnote); }
	.fold:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
	.fbar { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 8px; padding: 0 0 8px; min-width: 0; }
	.fbar .tfld { flex: 1 1 180px; max-width: 320px; }
	.tred { display: flex; flex-direction: column; gap: 8px; padding: 10px 0 2px; min-width: 0; }
	.tred .tl { margin: 0; display: flex; gap: 8px; align-items: baseline; flex-wrap: wrap; font: var(--t-footnote); }
	.trow { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; min-width: 0; }
	.tfld { flex: 1 1 200px; min-width: 0; box-sizing: border-box; min-height: 36px; padding: 7px 10px; border-radius: 8px; border: 1px solid var(--line-strong); background: var(--surface-2); color: var(--text); font: var(--t-footnote); resize: vertical; }
	.tfld.tier { flex: 0 0 72px; }
	.tfld:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	tr.fixed td { color: var(--text-muted); }
	.xt { display: grid; gap: 2px; margin: 0; min-width: 0; font: var(--t-footnote); }
	.xbn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.fn { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.lg { list-style: none; padding: 0; margin: 0; display: grid; gap: 3px; font: var(--t-caption); min-width: 0; max-height: 220px; overflow: auto; }
	.lg li { overflow-wrap: anywhere; }
	.lg time { color: var(--text-muted); font-variant-numeric: tabular-nums; margin-right: 6px; }
	.lg .bad { color: var(--fail-text); }
	.tblw { margin-top: 10px; max-width: 100%; overflow-x: auto; border: 1px solid var(--line); border-radius: 8px; }
	.fnm { display: inline-flex; align-items: center; gap: 4px; max-width: 100%; min-width: 0; }
	.fold { flex: none; display: inline-grid; place-items: center; width: 24px; height: 24px; padding: 0; border: 0; border-radius: 4px; background: none; color: var(--text-muted); cursor: pointer; }
	.fold.sp0 { cursor: default; }
	.fold :global(svg) { transition: transform var(--dur-fast) var(--ease-out); }
	.fold.shut :global(svg) { transform: rotate(-90deg); }
	.pbar { display: inline-block; vertical-align: middle; width: 56px; height: 5px; border-radius: 3px; background: var(--surface-3); overflow: hidden; }
	.pbar i { display: block; height: 100%; background: var(--ok); }
	.pv { display: inline-block; min-width: 34px; margin-left: 6px; text-align: right; }
	.pick { max-width: 88px; padding: 2px 4px; border: 1px solid var(--line); border-radius: 6px; background: var(--surface-1); color: var(--text); font: var(--t-footnote); }
	.pick:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	:global(.pb) .t2 { font: var(--t-callout); }
	:global(.pb) .t2 th { padding: 3px 8px; }
	:global(.pb) .t2 td { padding: 2px 8px; }
	:global(.pb) .tblw { margin-top: 6px; }
	:global(.pb) .lg { font: var(--t-callout); }
	.xbn { min-height: 32px; padding: 0 14px; border: 1px solid var(--surface-3); border-radius: var(--r-btn); background: var(--surface-2); color: var(--text); font: var(--t-footnote); font-weight: 500; cursor: pointer; }
	.xbn:hover:not(:disabled) { background: var(--surface-3); }
	.xbn:disabled { opacity: 0.5; cursor: not-allowed; }
	.pick, .fold { border-radius: var(--r-sm); }
	.tfld { border-radius: var(--r-btn); }
	:global(.pb) .tblw { margin-top: 0; max-height: max(160px, calc(var(--ph, 260px) - 150px)); overflow: auto; border-radius: var(--r-card); background: var(--surface-1); }
	.t2 { width: 100%; border-collapse: separate; border-spacing: 0; font-variant-numeric: tabular-nums; }
	:global(.pb) .t2 th { position: sticky; top: 0; z-index: 1; height: 36px; padding: 0 12px; border-bottom: 1px solid var(--line); background: var(--surface-1); color: var(--text-muted); font: var(--t-footnote); font-weight: 600; letter-spacing: 0.04em; text-transform: uppercase; text-align: left; white-space: nowrap; }
	:global(.pb) .t2 td { height: 36px; padding: 0 12px; border-bottom: 1px solid var(--line); vertical-align: middle; white-space: nowrap; transition: background-color var(--dur-fast) var(--ease-out); }
	.t2 tbody tr:last-child td { border-bottom: 0; }
	:global(.pb) .t2 .c-num { width: 1%; text-align: right; }
	:global(.pb) .t2 .c-chk { width: 32px; padding-right: 0; }
	.t2 .c-prog { width: 128px; }
	.t2 .c-prio { width: 1%; }
	.t2 .c-name, .t2 .c-msg { max-width: 0; width: 40%; }
	.t2 .c-name .fn, .t2 .c-msg .fn { display: block; }
	.t2 .c-name .fnm .fn { display: inline; }
	.t2 tbody tr { cursor: default; outline: none; }
	.t2 tbody tr:hover td { background: var(--surface-2); }
	.t2 tbody tr.sel td { background: color-mix(in srgb, var(--accent) 7%, var(--surface-1)); }
	.t2 tbody tr.sel td:first-child { box-shadow: inset 2px 0 0 var(--accent); }
	.t2 tbody tr:focus-visible td { box-shadow: inset 0 1px 0 var(--accent), inset 0 -1px 0 var(--accent); }
	.t2 tbody tr.off td { color: var(--text-muted); }
	:global(.pb) .tblw { border: 1px solid var(--line-strong); border-radius: var(--r-card); background: var(--surface-2); box-shadow: var(--shadow-card); }
	:global(.pb) .t2 th { background: color-mix(in srgb, var(--surface-3) 70%, var(--surface-2)); border-bottom-color: var(--line-strong); }
	:global(.pb) .t2 td { border-bottom-color: var(--line); }
	:global(.pb) .t2 tbody tr:nth-child(even) td { background: color-mix(in srgb, var(--surface-3) 28%, var(--surface-2)); }
	:global(.pb) .t2 tbody tr:hover td { background: var(--surface-3); }
	:global(.pb) .t2 tbody tr.sel td { background: color-mix(in srgb, var(--accent) 10%, var(--surface-2)); }
	:global(.pb) .t2 tbody tr { animation: fadein var(--dur) var(--ease-out); }
	:global(.pb) .t2 td.c-chk input[type='checkbox'] { appearance: none; display: inline-grid; place-content: center; width: 16px; height: 16px; margin: 0; border: 1.5px solid var(--line-strong); border-radius: 5px; background: var(--surface-1); cursor: pointer; transition: background-color var(--dur-fast) var(--ease-out), border-color var(--dur-fast) var(--ease-out); }
	:global(.pb) .t2 td.c-chk input[type='checkbox']::after { content: ''; width: 8px; height: 4px; border: solid #fff; border-width: 0 0 2px 2px; transform: translateY(-1px) rotate(-45deg) scale(0); transition: transform var(--dur-fast) var(--ease-out); }
	:global(.pb) .t2 td.c-chk input[type='checkbox']:checked { border-color: var(--accent); background: var(--accent); }
	:global(.pb) .t2 td.c-chk input[type='checkbox']:checked::after { transform: translateY(-1px) rotate(-45deg) scale(1); }
	:global(.pb) .t2 td.c-chk input[type='checkbox']:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	:global(.pb) .t2 td.c-chk input[type='checkbox']:disabled { opacity: 0.5; cursor: default; }
	:global(.pb) .t2 .pick { max-width: 120px; height: 28px; padding: 0 24px 0 8px; border: 1px solid var(--line-strong); border-radius: var(--r-sm); background: var(--surface-1) url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='6' viewBox='0 0 10 6'%3E%3Cpath d='M1 1l4 4 4-4' fill='none' stroke='%23888' stroke-width='1.6' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E") no-repeat right 8px center; color: var(--text); font: var(--t-callout); appearance: none; cursor: pointer; }
	:global(.pb) .t2 .pick:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.calm { display: flex; align-items: center; gap: 12px; margin: 8px 0 0; padding: 10px 12px; border: 1px solid var(--line); border-radius: var(--r-card); background: var(--surface-2); color: var(--text-muted); font: var(--t-footnote); }
	.calm.wait { border-color: transparent; background: none; padding-left: 0; }
	.calm .xbn { margin-left: auto; }
	.pbar { width: 64px; height: 4px; border-radius: var(--r-pill); }
	.pbar i { border-radius: inherit; transition: width var(--dur-slow) var(--ease-out); }
	.pv { min-width: 36px; margin-left: 8px; color: var(--text-muted); }
	.fbar { padding: 0 0 8px; gap: 8px; }
	:global(.pb) .lg { gap: 0; max-height: none; font: var(--t-callout); }
	.lg li { display: grid; grid-template-columns: max-content 16px minmax(0, 1fr); column-gap: 8px; align-items: start; padding: 4px 0; }
	.lg time { margin: 0; white-space: nowrap; }
	.lgd { position: relative; align-self: stretch; }
	.lgd::before { content: ''; position: absolute; left: 50%; top: -4px; bottom: -4px; width: 1px; margin-left: -0.5px; background: var(--line); }
	.lgd::after { content: ''; position: absolute; left: 50%; top: 5px; width: 7px; height: 7px; margin-left: -3.5px; border-radius: 50%; background: var(--surface-1); box-shadow: inset 0 0 0 1.5px var(--text-muted); }
	.lg li:first-child .lgd::before { top: 8px; }
	.lg li:last-child .lgd::before { bottom: auto; height: 12px; }
	.lg li:only-child .lgd::before { display: none; }
	.lg .bad .lgd::after { background: var(--fail); box-shadow: none; }
	.lgt { overflow-wrap: anywhere; }
	.quiet0 { margin: 0; padding: 10px 12px; border: 1px solid var(--line-strong); border-radius: var(--r-card); background: var(--surface-2); box-shadow: var(--shadow-card); color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; }
	.lgw { display: grid; gap: 4px; }
	.lgday { margin: 6px 0 0; color: var(--text-muted); font: var(--t-caption); font-weight: 600; }
	.lgday:first-child { margin-top: 0; }
	.lgw { gap: 0; border: 1px solid var(--line); border-radius: var(--r-card, 10px); background: var(--surface-1); box-shadow: 0 1px 2px rgb(0 0 0 / 0.06), 0 6px 16px rgb(0 0 0 / 0.06); overflow: hidden; min-width: 0; }
	.lgday { margin: 0; padding: 8px 14px; background: color-mix(in srgb, var(--surface-2) 70%, var(--surface-1)); border-bottom: 1px solid var(--line); color: var(--text-muted); font: var(--t-footnote); font-weight: 600; letter-spacing: 0.04em; text-transform: uppercase; }
	.lgday:not(:first-child) { border-top: 1px solid var(--line); }
	.lgw .lg { gap: 0; padding: 4px 14px; max-height: none; }
	.lgw .lg li { padding: 8px 0; border-bottom: 1px solid color-mix(in srgb, var(--line) 60%, transparent); }
	.lgw .lg li:last-child { border-bottom: 0; }
	.lgw .lg time { color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.fbar .tfld { min-height: 34px; padding: 7px 12px; border: 1px solid var(--line); border-radius: var(--r-card, 10px); background: var(--surface-1); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.04), 0 1px 2px rgb(0 0 0 / 0.05); transition: border-color 150ms var(--ease-out), box-shadow 150ms var(--ease-out); }
	.fbar .tfld:hover { border-color: var(--line-strong); }
	.fbar .tfld:focus-visible { border-color: var(--line-strong); box-shadow: var(--focus-ring); }
	.facts { justify-self: start; width: fit-content; max-width: 100%; box-sizing: border-box; min-width: 0; padding: 4px 0; border: 1px solid var(--line-strong); border-radius: var(--r-card); background: var(--surface-2); box-shadow: var(--shadow-card); overflow: hidden; }
	.fcap { margin: 0; padding: 10px 12px 2px; color: var(--text-muted); font: var(--t-footnote); font-weight: 600; letter-spacing: 0.06em; text-transform: uppercase; }
	.fcols { display: grid; grid-template-columns: repeat(var(--fn, 4), minmax(0, 420px)); }
	.fcol { min-width: 0; padding: 0 0 6px; }
	.fcol + .fcol { border-left: 1px solid var(--line); }
	.fgrid { display: block; margin: 0; padding: 0 16px; }
	.fr { display: flex; align-items: center; gap: 16px; min-width: 0; height: 36px; box-sizing: border-box; border-bottom: 1px solid var(--line); font: var(--t-callout); font-variant-numeric: tabular-nums; }
	.fr dt { flex: none; color: var(--text-muted); }
	.fr dd { flex: 1 1 auto; min-width: 0; margin: 0; text-align: right; font-variant-numeric: tabular-nums; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.fr dd.path { direction: rtl; }
	.fr:last-child { border-bottom-color: transparent; }
	.prf { flex-direction: column; align-items: flex-start; gap: 8px; margin: 0 0 8px; }
	.hlink { display: inline-block; margin: 6px 12px 2px; color: var(--accent-text); font: var(--t-footnote); }
	.fnone { margin: 0; padding: 10px 12px; color: var(--text-muted); font: var(--t-callout); }
	.calm.slim { margin: 4px 8px 4px; padding: 4px 4px 4px 10px; gap: 8px; border-color: var(--line); background: color-mix(in srgb, var(--surface-3) 50%, var(--surface-2)); font: var(--t-caption); }
	.calm.slim .xbn { min-height: 24px; }
	.tblw::-webkit-scrollbar, .lg::-webkit-scrollbar { width: 12px; height: 12px; background: transparent; }
	.tblw::-webkit-scrollbar-track, .lg::-webkit-scrollbar-track { margin: 4px; border: 3px solid transparent; border-radius: 12px; background: color-mix(in srgb, var(--text) 6%, transparent) padding-box; }
	.tblw::-webkit-scrollbar-corner { background: transparent; }
	.tblw::-webkit-scrollbar-button, .lg::-webkit-scrollbar-button { display: none; width: 0; height: 0; }
	.tblw::-webkit-scrollbar-thumb, .lg::-webkit-scrollbar-thumb { border: 3px solid transparent; border-radius: 12px; background: color-mix(in srgb, var(--text) 26%, transparent) padding-box; }
	.tblw::-webkit-scrollbar-thumb:hover, .lg::-webkit-scrollbar-thumb:hover { background: color-mix(in srgb, var(--text) 40%, transparent) padding-box; }
	.facts { width: auto; justify-self: stretch; padding: 2px 0; border-color: var(--line); border-radius: var(--r-card); background: color-mix(in srgb, var(--surface-1) 72%, transparent); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 6px 20px -10px rgb(0 0 0 / 0.35); }
	.fcols { grid-template-columns: repeat(auto-fit, minmax(min(100%, 260px), 1fr)); align-items: start; overflow: hidden; }
	.fcol + .fcol { border-left: 0; }
	.fcol { border-right: 1px solid var(--line); margin-right: -1px; }
	.fcap { padding: 12px 16px 4px; letter-spacing: 0; text-transform: none; }
	:global(.pb) .t2 th, .lgday { letter-spacing: 0; text-transform: none; }
	.hlink { margin: 10px 16px 2px; font-weight: 600; }
	:global(.pb) .lgw, :global(.pb) .tblw, :global(.pb) .quiet0 { border-radius: var(--r-card); border-color: var(--line); }
	@media (prefers-reduced-motion: reduce) {
		:global(.pb) .t2 tbody tr { animation: none; }
		.t2 td, .pbar i { transition: none; }
	}
	@media (max-width: 640px) {
		:global(.pb) .tblw { max-height: 40dvh; }
		.fold { width: 32px; height: 32px; }
		.xbn, .calm.slim .xbn { min-height: 32px; }
		:global(.pb) .t2 td.c-chk input[type='checkbox'] { width: 24px; height: 24px; }
		:global(.pb) .t2 .c-chk { width: 40px; }
	}
	@media (min-width: 1500px) {
		.fcols { grid-template-columns: repeat(var(--fn, 4), minmax(240px, 420px)); }
	}
	@media (max-width: 1100px) and (min-width: 768px) {
		.fcols { grid-template-columns: repeat(min(var(--fn, 2), 2), minmax(0, 420px)); }
		.fcol:nth-child(odd) { border-left: 0; }
		.fcol:nth-child(even) { border-left: 1px solid var(--line); }
		.fcol:nth-child(n + 3) { border-top: 1px solid var(--line); }
	}
	@media (max-width: 767px) {
		.fcols { grid-template-columns: minmax(0, 1fr); }
		.fcol + .fcol { border-left: 0; border-top: 1px solid var(--line); }
		.facts { width: auto; justify-self: stretch; }
		.fcol { border-right: 0; margin-right: 0; }
		.fcol + .fcol { border-top: 1px solid var(--line); }
	}
	@supports not selector(::-webkit-scrollbar) {
		.tblw, .lg { scrollbar-width: thin; scrollbar-color: color-mix(in srgb, var(--text) 24%, transparent) transparent; }
		.tblw:hover, .lg:hover { scrollbar-color: color-mix(in srgb, var(--text) 38%, transparent) transparent; }
	}
	@media (min-width: 641px) {
	:global(.dpane) .facts { background: transparent; box-shadow: none; border-color: transparent; }
	}
</style>
