<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Poster from '#lib/ui/Poster.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Menu from '#lib/ui/Menu.svelte';
	import type { MenuItem } from '#lib/ui/Menu.svelte';
	import { quality, ratioText, swarmNum, type ColId } from './columns.ts';
	import { rowItems } from './rowmenu.ts';
	import { tip } from './tip.ts';
	import { clientMark, swarmTotal } from './panel-words.ts';
	import { size as bytes, categoryWords, protocolOf, KIND_WORD } from './size.ts';
	import { kindOf, kindWords, manyWords, rowName } from './downloaders.ts';
	import type { ImportRecord } from './types.ts';
	import { shortReason, stageOf, statusHint, statusWord, stepLine } from './status-tabs.ts';
	import { statusDetail } from './imports.ts';
	import { poster, who } from './row-art.ts';
	import { longPress, phoneTitle, subLine } from './phone.ts';
	import { problem } from './problem.ts';
	import { barTone, historyStatus, moving } from './sections.ts';
	import type { Columns } from './columns-grid.svelte.ts';
	import { to } from '../core/paths.ts';
	import { ago, duration, percent } from '../core/format.ts';
	import { session } from '../core/session.svelte.ts';
	import type { DownloadRow, TitleView } from '../core/types.ts';

	type Props = {
		r: DownloadRow;
		hist: boolean;
		cols: Columns;
		watch: boolean;
		picked: Set<number>;
		choosing: boolean;
		openId: number;
		tabRow: number;
		importOf: Map<number, ImportRecord>;
		titles: Map<number, TitleView>;
		titlesReady: boolean;
		rowMenu: (r: DownloadRow) => MenuItem[];
		onpickone: (id: number, on?: boolean) => void;
		onpickto: (id: number) => void;
		onrowclick: (e: MouseEvent, id: number) => void;
		onrowkey: (e: KeyboardEvent, id: number) => void;
		onrowfocus: (id: number) => void;
	};
	let { r, hist, cols, watch, picked, choosing, openId, tabRow, importOf, titles, titlesReady, rowMenu, onpickone, onpickto, onrowclick, onrowkey, onrowfocus }: Props = $props();

	// Downloaders with a bundled app icon.
	const CLIENT_ICONS = new Set(['qbittorrent', 'transmission', 'deluge', 'sabnzbd', 'nzbget']);
	const dash = '—';
	const dlName = (r: DownloadRow) => r.downloader || (r.downloader_kind && kindWords[r.downloader_kind as keyof typeof kindWords]) || dash;
	const dlKind = (r: DownloadRow) => (r.downloader_kind && kindWords[r.downloader_kind as keyof typeof kindWords]) || '';
	const sub2 = (r: DownloadRow) => (manyWords(r.kind) && r.covers ? r.covers : r.name);
</script>

{#snippet lead(r: DownloadRow)}
	<span class="tg">{#if !watch}<input type="checkbox" class="pk" class:any={choosing} tabindex="-1" checked={picked.has(r.id)} aria-label={t('downloads.row.select', { name: who(r) })} onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); e.stopPropagation(); onpickone(r.id); } else if (e.key === ' ') e.stopPropagation(); }} onclick={(e) => { e.stopPropagation(); if (e.shiftKey) { e.preventDefault(); onpickto(r.id); } else onpickone(r.id, e.currentTarget.checked); }} data-testid="dl-pick" />{/if}</span>
{/snippet}

{#snippet cell(r: DownloadRow, id: ColId, ph: boolean, hist: boolean)}
	{#if id === 'poster'}
		<span class="post" class:square={r.kind === 'artist'} class:ph data-col="poster"><Poster src={poster(r, titles)} name={titlesReady ? r.title || r.name.replace(/[._]+/g, ' ') : ''} /></span>
	{:else if id === 'kind'}
		{@const kw = kindOf(r) ? KIND_WORD[kindOf(r) as 'movie'] : categoryWords(r.category)}{@const ki = cols.kindIcon(kw)}<span class="kcell" class:ph data-col="kind"><span class="kt" class:none={!ki} class:wide={cols.kindWide} data-testid="dl-kind" role="img" aria-label={kw || t('downloads.row.noType')} use:tip={kw ? (kindOf(r) ? kw : t('downloads.row.fromCategory', { kw })) : t('downloads.row.noType')}>{#if cols.kindWide && kw}<span class="kp" data-k={ki ?? 'other'}>{kw}</span>{:else if ki}<Icon name={ki} size={16} />{:else}{dash}{/if}</span></span>
	{:else if id === 'proto'}
		{@const p = protocolOf(r)}
		<span class="pcell" class:ph class:named={cols.srcName} data-col="proto">{#if p}{@const cm = clientMark(r.downloader_kind)}{@const pw = p === 'torrent' ? 'Torrent' : 'Usenet'}{@const logo = CLIENT_ICONS.has(r.downloader_kind ?? '')}<span class="pm" class:logo class:cmk={!logo && !!cm.mark} data-tone={logo ? undefined : cm.tone || undefined} data-testid="dl-protocol" data-protocol={p} data-kind={r.downloader_kind} use:tip={`${dlName(r)} · ${pw}`} role="img" aria-label="{dlName(r)} · {pw}">{#if logo}<img class="cico" src={to(`/clients/${r.downloader_kind}.svg`)} alt="" width="24" height="24" decoding="async" />{:else if cm.mark}{cm.mark}{:else}<Icon name={p === 'torrent' ? 'torrent' : 'usenet'} size={14} />{/if}</span>{#if cols.srcName}<span class="pnm" title={dlName(r)}>{dlKind(r) || dlName(r)}</span>{/if}{/if}</span>
	{:else if id === 'title'}
		<span class="nm" class:ph data-col="title">
			<span class="tt" title={rowName(r)}><span class="tn">{rowName(r)}</span><span class="tphone">{phoneTitle(r)}</span>{#if r.version && r.version !== 'Main'}<span class="ver">{r.version}</span>{/if}</span>
			<span class="rel" title={r.name}>{sub2(r)}</span>
			{#if subLine(r)}<span class="ep" data-testid="dl-epline">{subLine(r)}</span>{/if}
		</span>
	{:else if id === 'progress'}
			<!-- A finished row shows a full bar; nothing stops at 99. -->
			{@const hs = hist && stageOf(r) !== 'failed' ? historyStatus(r, importOf.get(r.id)) : null}
			{@const raw = percent(r.progress)}
			{@const pct = hs && !hs.bad ? 100 : raw}
			{@const tone = hs ? (hs.bad ? 'bad' : 'done') : stageOf(r) === 'paused' ? 'paused' : barTone(r)}
			{@const dn = (r.speed ?? 0) >= 1024}
			{@const upOn = (r.up_speed ?? 0) >= 1024}
			{@const eta = r.active && r.eta ? duration(r.eta) : ''}
			<span class="pgw" class:ph data-col="progress"><span class="prg {tone}" role="progressbar" aria-label={t('downloads.row.progressOf', { name: who(r) })} aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.floor(pct)} data-testid="dl-bar"><i style="transform: translateX({pct - 100}%)"></i><span class="bl">{Math.floor(pct)}%</span></span>{#if moving(r) && (dn || upOn || eta)}<span class="spd" data-testid="dl-row-speed">{[dn ? `↓ ${bytes(r.speed)}/s` : '', upOn ? `↑ ${bytes(r.up_speed)}/s` : '', eta ? t('downloads.list.left', { time: eta }) : ''].filter(Boolean).join(' · ')}</span>{/if}</span>
	{:else if id === 'status'}
		{#if hist}
			{@const s = historyStatus(r, importOf.get(r.id))}
			{@const cut = s.text.indexOf(': ')}
			{@const reason = cut > 0 ? shortReason(s.text.slice(cut + 2)) : ''}
			<span class="st" class:ph class:bad={s.bad} class:two={!!reason} data-col="status" title={s.text}><span class="dot" aria-hidden="true"></span><span class="tx"><span class="sw">{cut > 0 ? s.text.slice(0, cut) : s.text}</span>{#if reason}<span class="why" data-testid="dl-why">{reason}</span>{/if}</span></span>
		{:else}
			{@const tn = barTone(r)}
			{@const detail = statusDetail(r, importOf.get(r.id))}
			{@const more = detail || statusHint(r)}
			{@const why = problem(r)}
			<!-- The line shows the problem, else the queue place, else the current step. -->
			{@const line = why ? '' : stageOf(r) === 'queued' && r.queue_pos ? (r.queue_pos === 1 ? t('downloads.row.nextInLine') : t('downloads.row.inLine', { ordinal: cols.ordinal(r.queue_pos) })) : stepLine(r, detail)}
			{@const under = why?.text || line}
			<span class="st {tn}" class:ph class:two={!!under} data-col="status" title={[statusWord(r), under, more.toLowerCase() === under.toLowerCase() ? '' : more].filter(Boolean).join(' · ')}><span class="dot" aria-hidden="true"></span><span class="tx"><span class="sw">{statusWord(r)}</span>{#if why}<span class="why" data-testid="dl-why">{why.text.charAt(0).toUpperCase() + why.text.slice(1)}</span>{:else if line}<span class="why" data-testid={stageOf(r) === 'queued' ? 'dl-in-line' : 'dl-step'}>{line}</span>{/if}</span></span>
		{/if}
	{:else if id === 'size'}
		<span class="num" class:ph data-col="size">{r.size ? bytes(r.size) : dash}</span>
	{:else if id === 'quality'}
		<span class="txt" class:ph data-col="quality" title={r.name}>{quality(r.name) || dash}</span>
	{:else if id === 'added'}
		<span class="num" class:ph data-col="added" title={r.started_at ?? ''}>{r.started_at ? ago(r.started_at) : dash}</span>
	{:else if id === 'swarm'}
		{#if r.protocol !== 'torrent'}<span class="num" class:ph data-col="swarm">{dash}</span>
		{:else}
			{@const st = swarmTotal(r.seeds, r.seeds_total)}
			{@const pt = swarmTotal(r.peers, r.peers_total)}
			{@const tipText = [st ? t('downloads.row.seedsSwarm', { n: st }) : '', pt ? t('downloads.row.peersSwarm', { n: pt }) : ''].filter(Boolean).join(' · ') || t('downloads.row.connected')}
			<!-- Seeds, peers and ratio on one line; the ratio turns green from 1.0. -->
			<span class="sw1" class:ph data-col="swarm" title={t('downloads.row.ratio', { tip: tipText, ratio: r.ratio != null ? r.ratio.toFixed(2) : '–' })}><span class="n"><i class="dot s" aria-hidden="true"></i>{swarmNum(r.seeds)}</span><span class="n"><i class="dot p" aria-hidden="true"></i>{swarmNum(r.peers)}</span>{#if r.ratio != null}<b class="rp" class:good={r.ratio >= 1}>{ratioText(r.ratio)}</b>{/if}</span>
		{/if}
	{:else if id === 'tracker'}
		<span class="txt" class:ph data-col="tracker" title={r.tracker ?? ''}>{r.tracker || dash}</span>
	{:else if id === 'requested'}
		<!-- A person, HolyArrs itself (a title it grabbed on its own), or Outside (added in the client). -->
		{@const person = !r.outside && r.requested_by ? r.requested_by : ''}
		{@const who = r.outside ? t('downloads.row.outside') : person || (session.can('manage_library') ? 'HolyArrs' : '')}
		<span class="txt rq" class:ph class:soft={!person} data-col="requested" title={who}>{#if person}<i class="av" aria-hidden="true">{person.slice(0, 1).toUpperCase()}</i>{/if}{who || dash}</span>
	{:else if id === 'source'}
		<span class="txt" class:ph data-col="source" title={r.source_name ?? ''}>{#if r.source_name && r.source_id && session.can('settings')}<a href="/settings/sources/{r.source_id}" onclick={(e) => e.stopPropagation()}>{r.source_name}</a>{:else}{r.source_name || dash}{/if}</span>
	{:else if id === 'category'}
		{@const cw = categoryWords(r.category)}<span class="txt" class:ph data-col="category" title={r.category ?? ''}>{cw || dash}</span>
	{:else}
		<span class="num" class:ph data-col={id}>{dash}</span>
	{/if}
{/snippet}

{#snippet bin(r: DownloadRow)}
	<span class="del" data-col="bin">
		{#if watch}{:else if rowItems(r).length}<span class="rm" data-testid="dl-row-menu"><Menu label={t('downloads.row.actionsFor', { name: who(r) })} icon="more" width={210} items={rowMenu(r)} tabindex={tabRow === r.id ? 0 : -1} /></span>
		{:else}<button class="bin" type="button" disabled title={t('downloads.row.cannotChange')} aria-label={t('downloads.row.cannotChange')} tabindex={tabRow === r.id ? 0 : -1}><Icon name="more" size={19} /></button>{/if}
	</span>
{/snippet}

{#snippet row(r: DownloadRow, hist: boolean)}
	<li class="drow" class:sel={openId === r.id} class:picked={picked.has(r.id)} data-testid="dl-table-row" data-dl-id={r.id} data-phase={r.phase}>
		<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions, a11y_no_static_element_interactions -->
		<div class="dline" class:hist tabindex={tabRow === r.id ? 0 : -1} onfocus={() => onrowfocus(r.id)} aria-current={openId === r.id} aria-label={t('downloads.row.showDetails', { name: who(r) })} data-testid="dl-line" use:longPress={() => { if (!watch) onpickone(r.id, true); }} onclick={(e) => onrowclick(e, r.id)} onkeydown={(e) => onrowkey(e, r.id)}>
			{@render lead(r)}
			{#each cols.cells as c (c.id)}{@render cell(r, c.id, c.ph, hist)}{/each}
			<span class="spare"></span>
			{@render bin(r)}
		</div>
	</li>
{/snippet}

{@render row(r, hist)}

<style>
	.pm.logo, .pm:has(.cico) { width: auto; height: auto; padding: 0; border: 0; border-radius: 0; background: none; box-shadow: none; }
	.cico { display: block; flex: none; width: var(--lg, 18px); height: var(--lg, 18px); object-fit: contain; }
	.pcell.named { justify-content: flex-start; gap: 6px; }
	.pcell.named > .pm { flex: none; }
	.pnm { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); font: var(--t-footnote); }
	.types { display: inline-flex; gap: 2px; padding: 2px; margin-bottom: 12px; border-radius: var(--r-btn); background: var(--surface-2); }
	.pm { display: inline-grid; place-items: center; width: 14px; height: 14px; margin-right: 6px; vertical-align: 1px; border-radius: 3px; background: var(--surface-3); color: var(--text-muted); font-size: 9px; font-weight: 700; line-height: 1; }
	.kt { display: inline-block; box-sizing: border-box; height: 16px; margin-right: 6px; padding: 0 5px; vertical-align: 1px; border: 1px solid var(--line); border-radius: var(--r-sm); background: var(--surface-3); color: var(--text); font: var(--t-caption); font-size: 11px; line-height: 14px; font-weight: 600; white-space: nowrap; }
	.drow { display: block; border-bottom: 1px solid var(--line); }
	.tg { position: relative; align-self: stretch; }
	.pk { position: absolute; left: 4px; top: 50%; width: 16px; height: 16px; margin: -8px 0 0; accent-color: var(--accent); opacity: 0; cursor: pointer; z-index: 2; transition: opacity 120ms var(--ease-out); }
	.drow:hover .pk, .pk.any, .pk:focus-visible { opacity: 1; }
	.drow.picked > .dline { background: color-mix(in srgb, var(--accent) 9%, transparent); }
	.tt .tphone, .nm .ep { display: none; }
	.dline { display: grid; grid-template-columns: var(--tpl); align-items: center; column-gap: var(--gap); height: max(44px, calc(var(--pi, 26px) * 1.5 + 6px)); }
	.drow > .dline { cursor: pointer; }
	.drow > .dline:hover { background: var(--surface-2); }
	.drow > .dline:focus-visible { outline: none; box-shadow: inset var(--focus-ring); }
	.bin:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.post { width: var(--pi, 26px); height: calc(var(--pi, 26px) * 1.5); justify-self: center; overflow: hidden; border-radius: 3px; --poster-ratio: 2 / 3; }
	.post.square { height: var(--pi, 26px); --poster-ratio: 1; }
	.post :global(.poster) { border-radius: 3px; }
	.nm { display: flex; flex-direction: column; min-width: 0; line-height: 1.25; }
	.tt, .rel { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.tt { font: var(--t-callout); font-weight: 500; display: flex; align-items: center; min-width: 0; }
	.tt .tn, .tt .tphone { min-width: 0; overflow: hidden; text-overflow: ellipsis; }
	.tt .ver { flex: none; }
	.rel { color: var(--text-muted); font: var(--t-caption); }
	.ver { margin-left: 6px; padding: 0 5px; border-radius: 4px; background: var(--surface-3); color: var(--text-muted); font: var(--t-caption); }
	.prg { position: relative; display: flex; align-items: center; height: 14px; white-space: nowrap; padding: 0 6px; border-radius: 4px; overflow: hidden; background: var(--surface-3); color: var(--text); font-size: 10px; line-height: 14px; font-weight: 600; font-variant-numeric: tabular-nums; }
	.prg i { position: absolute; inset: 0; background: var(--ok); opacity: 0.5; transition: transform var(--dur-slow) var(--ease-out); }
	.prg.queued i { background: var(--info); }
	.prg:global(.held) i { background: var(--wait); }
	.prg.bad i { background: var(--text-muted); opacity: 0.3; }
	.bl { position: relative; overflow: hidden; text-overflow: ellipsis; }
	.bl { font-weight: 600; text-shadow: 0 0 3px var(--surface-1); }
	.st { display: flex; align-items: center; gap: 6px; min-width: 0; font: var(--t-footnote); }
	.st .tx { display: flex; flex-direction: column; gap: 1px; min-width: 0; }
	.st .sw, .st .why { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.st .why { color: var(--text-muted); font: var(--t-caption); }
	.st.two { align-items: flex-start; }
	.st.two .dot { margin-top: 4px; }
	.rq { display: flex; align-items: center; gap: 6px; }
	.rq.soft { color: var(--text-muted); }
	.rq .av { display: grid; place-items: center; flex: none; width: 18px; height: 18px; border-radius: 50%; background: var(--text); color: var(--bg); font: 600 10px / 1 var(--font-ui); font-style: normal; }
	.st .dot { flex: none; width: 8px; height: 8px; border-radius: 50%; background: var(--ok); }
	.st.paused .dot { background: var(--text-muted); }
	.st.queued .dot { background: var(--info); }
	.st:global(.held) .dot { background: var(--wait); }
	.st.bad .dot { background: var(--fail); }
	.st.bad { color: var(--fail-text); }
	.dline > .ph { display: none; }
	.txt { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); font: var(--t-footnote); }
	.num { text-align: right; color: var(--text-muted); font: var(--t-footnote); font-variant-numeric: tabular-nums; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.del { display: flex; justify-content: flex-end; }
	.bin { display: grid; place-items: center; width: 32px; height: 32px; padding: 0; border: 0; border-radius: 6px; background: none; color: var(--text-muted); cursor: pointer; transition: color var(--dur-fast) var(--ease-out), background var(--dur-fast) var(--ease-out); }
	.bin:hover:not(:disabled) { color: var(--fail-text); background: var(--surface-3); }
	.bin:disabled { opacity: 0.35; cursor: not-allowed; }
	.post { transition: opacity var(--dur-fast) var(--ease-out); }
	.dot { width: 8px; height: 8px; border-radius: 50%; background: var(--wait); }
	.dot:global(.ok) { background: var(--ok); box-shadow: 0 0 0 3px color-mix(in srgb, var(--ok) 25%, transparent); }
	.drow.sel > .dline { position: relative; z-index: 1; background: var(--surface-2); box-shadow: inset 3px 0 0 var(--accent), 0 5px 10px -6px rgb(0 0 0 / 0.4), 0 -5px 10px -6px rgb(0 0 0 / 0.22); }
	.drow.sel > .dline:hover { background: var(--surface-2); }
	.pt { flex: 0 1 380px; min-width: 0; margin: 0; }
	.pt span { font: var(--t-callout); }
	.pt span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.kcell, .pcell { display: flex; align-items: center; min-width: 0; }
	.kcell .kt.none { color: var(--text-muted); }
	.kcell .kt.wide { justify-content: flex-start; }
	.kp { padding: 1px 7px; border-radius: 999px; font: var(--t-caption); font-weight: 600; color: color-mix(in srgb, var(--kc) 70%, var(--text)); background: color-mix(in srgb, var(--kc) 16%, transparent); overflow: hidden; text-overflow: ellipsis; }
	.kp[data-k='film'] { --kc: var(--wait); }
	.kp[data-k='tv'] { --kc: var(--info); }
	.kp[data-k='music'] { --kc: var(--ok); }
	.kp[data-k='other'] { --kc: var(--text-muted); }
	.kcell .kt { display: flex; justify-content: center; align-items: center; height: auto; padding: 0; border: 0; background: none; width: 100%; margin: 0; text-align: center; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.pcell { justify-content: center; }
	.pm { display: inline-grid; place-items: center; width: 22px; height: 22px; margin: 0; border-radius: 6px; background: var(--surface-3); color: var(--text-muted); }
	.prg.paused i { background: var(--text-muted); }
	.pgw { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
	.spd { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); font: var(--t-caption); font-variant-numeric: tabular-nums; }
	.dline :global(.num), .num { text-align: right; font-variant-numeric: tabular-nums; }
	.pw { display: flex; align-items: center; flex: none; border-radius: 999px; background: transparent; transition: background-color 200ms var(--ease-out), box-shadow 200ms var(--ease-out); }
	.more { min-width: 36px; padding: 0 10px; }
	.post, .post :global(.poster) { border-radius: var(--r-poster); }
	.drow > .dline { transition: background-color var(--dur-fast) var(--ease-out); }
	.drow.sel > .dline { background: var(--surface-1); box-shadow: inset 3px 0 0 var(--accent), var(--shadow-card); }
	.drow.sel > .dline:hover { background: var(--surface-1); }
	.drow { animation: rowin var(--dur) var(--ease-out) backwards; }
	:global(.dlrows):global(.vw) .drow { animation: none; }
	.st { gap: 8px; }
	.prg { height: 16px; padding: 0 8px; border-radius: var(--r-pill); font-size: 10px; line-height: 16px; }
	.bin { border-radius: var(--r-sm); }
	.pt span { font-variant-numeric: tabular-nums; }
	.pm.cmk { display: inline-grid; place-items: center; width: auto; min-width: 22px; height: 22px; padding: 0 3px; border-radius: 5px; font: 700 9px/1 var(--font-ui); letter-spacing: 0.02em; color: var(--cm); background: color-mix(in srgb, var(--cm) 16%, transparent); box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--cm) 40%, transparent); }
	.pm[data-tone='qb'] { --cm: #2f6fd6; }
	.pm[data-tone='tr'] { --cm: #d23c3c; }
	.pm[data-tone='de'] { --cm: #5b7388; }
	.pm[data-tone='sab'] { --cm: #a87a00; }
	.pm[data-tone='nzb'] { --cm: #2e8b4a; }
	:global([data-theme='dark']) .pm[data-tone='qb'] { --cm: #7aa8f5; }
	:global([data-theme='dark']) .pm[data-tone='tr'] { --cm: #f07a7a; }
	:global([data-theme='dark']) .pm[data-tone='de'] { --cm: #9db3c6; }
	:global([data-theme='dark']) .pm[data-tone='sab'] { --cm: #f0c84a; }
	:global([data-theme='dark']) .pm[data-tone='nzb'] { --cm: #6cd08c; }
	.sw1 { display: grid; grid-template-columns: 42px 42px auto; column-gap: 4px; align-items: center; justify-content: start; }
	.sw1 { min-width: 0; font: var(--t-footnote); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.sw1 .n { display: inline-flex; align-items: center; gap: 4px; }
	.sw1 .dot { flex: none; width: 7px; height: 7px; border-radius: 50%; background: var(--ok); }
	.sw1 .dot.p { background: var(--info); }
	.sw1 .rp { justify-self: start; padding: 1px 6px; border-radius: 999px; background: var(--surface-3); color: var(--text-muted); font: var(--t-caption); font-weight: 500; font-variant-numeric: tabular-nums; }
	.sw1 .rp.good { background: color-mix(in srgb, var(--ok) 18%, transparent); color: var(--ok-text); }
	.pt { gap: 3px; padding: 8px 0; }
	@media (min-width: 641px) {
		.pcell:not(.named) > .pm, .dline > .post, .kcell > .kt:not(.wide) { transform: translateX(calc(var(--gap) / 2 - 4px)); }
		:global(.dlrows) .drow:last-child { border-bottom: 0; }
		:global(.dlrows) .drow:nth-child(even) > .dline { background: color-mix(in srgb, var(--surface-3) 28%, var(--surface-2)); }
		:global(.dlrows) .drow > .dline:hover { background: var(--surface-3); box-shadow: 0 2px 8px -4px rgb(0 0 0 / 0.25); }
		:global(.dlrows) .drow.picked > .dline { background: color-mix(in srgb, var(--accent) 9%, var(--surface-2)); }
		:global(.dlrows) .drow.sel > .dline, :global(.dlrows) .drow.sel > .dline:hover { background: var(--surface-1); box-shadow: inset 3px 0 0 var(--accent), var(--shadow-card); }
	.tg:has(> .pk) { display: flex !important; align-items: center; justify-content: center; min-width: 28px; padding: 0 !important; }
		.tg > .pk { margin: 0; translate: none; transform: none; }
	.tg > .pk { left: 6px; top: calc(50% - 8px); }
	.dline > .post { transform: translateX(calc(var(--gap) / 2 + 0.5px)); }
	}
	@media (hover: none) {
		.pk { opacity: 1; }
	}
	@media (prefers-reduced-motion: reduce) {
		.pw { transition: none; }
		.drow { animation: none; }
		.drow > .dline { transition: none; }
		.prg i, .tg :global(svg), .bin, .post { transition: none; }
	}
	@media (max-width: 640px) {
	.dline { grid-template-columns: 8px 36px minmax(0, 1fr) 40px auto 44px; grid-template-areas: 'tg post nm nm nm mn' 'tg post st bar time mn'; grid-template-rows: auto 18px; column-gap: 10px; row-gap: 2px; height: auto; min-height: 64px; padding: 4px 12px 4px 0; box-sizing: border-box; align-items: center; -webkit-touch-callout: none; -webkit-user-select: none; user-select: none; -webkit-tap-highlight-color: transparent; transition: background-color 160ms var(--ease-out), transform 160ms var(--ease-out); }
		.dline:global([data-pressing]) { transform: scale(0.985); background: color-mix(in srgb, var(--text) 7%, transparent); transition-duration: 450ms; }
		.drow.picked > .dline { background: color-mix(in srgb, var(--accent) 10%, transparent); }
		.dline > [data-col] { display: none; }
		.dline > .ph { display: none; }
		.dline > [data-col='poster'] { display: block; grid-area: post; align-self: start; }
		.dline > [data-col='title'] { display: flex; grid-area: nm; }
		.dline > [data-col='progress'] { display: flex; grid-area: bar; }
		.dline > [data-col='status'] { display: flex; grid-area: st; }
		.dline > [data-col='time'] { display: block; grid-area: time; }
		.dline > [data-col='title'] { flex-direction: column; justify-content: center; gap: 1px; min-width: 0; }
		.dline > [data-col='title'] .tn { display: none; }
		.dline > [data-col='title'] .tphone { display: inline; }
		.dline > [data-col='title'] .ep { display: block; overflow: hidden; color: var(--text-muted); font: 400 13px/16px var(--font-ui); text-overflow: ellipsis; white-space: nowrap; }
		.dline > [data-col='time'] { justify-self: end; color: var(--text-muted); font: var(--t-footnote); }
	.dline > [data-col='bin'] { display: flex; grid-area: mn; align-self: center; justify-content: center; }
	.tg .pk:not(.any) { width: 1px; height: 1px; margin: 0; opacity: 0; clip-path: inset(50%); overflow: hidden; pointer-events: none; }
		.dline > [data-col='poster'] { width: 36px; height: 54px; }
		.dline > [data-col='title'] .rel { display: none; }
		.dline > [data-col='title'] .tt { font: 600 15px/19px var(--font-ui); }
		.dline > [data-col='status'] { font: 400 12px/16px var(--font-ui); }
		.dline > [data-col='time'] { font-variant-numeric: tabular-nums; white-space: nowrap; }
		.tg { align-self: center; }
		.tg .pk.any { width: 20px; height: 20px; margin-left: 4px; }
		.dline:has(.pk.any) { grid-template-columns: 32px 36px minmax(0, 1fr) 40px auto 44px; }
		.dline > [data-col='progress'] { height: 4px; align-self: center; }
		.dline > [data-col='progress'] .bl, .spd { display: none; }
		.dline > .spare { display: none; }
		.tg { grid-area: tg; }
		.post { grid-area: post; align-self: start; justify-self: center; }
		.pt { flex-basis: 100%; }
	}
	@media (prefers-color-scheme: dark) {
		:global(:root:not([data-theme='light'])) .pm[data-tone='qb'] { --cm: #7aa8f5; }
		:global(:root:not([data-theme='light'])) .pm[data-tone='tr'] { --cm: #f07a7a; }
		:global(:root:not([data-theme='light'])) .pm[data-tone='de'] { --cm: #9db3c6; }
		:global(:root:not([data-theme='light'])) .pm[data-tone='sab'] { --cm: #f0c84a; }
		:global(:root:not([data-theme='light'])) .pm[data-tone='nzb'] { --cm: #6cd08c; }
	}
	@media (pointer: coarse) {
		.bin:disabled { pointer-events: none; }
	}
	.dline :global(.num) { text-align: right; font-variant-numeric: tabular-nums; }
</style>
