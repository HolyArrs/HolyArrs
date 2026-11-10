<script lang="ts">
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Avatar from '#lib/ui/Avatar.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { factsLine, filterOf, fixReport, kindHint, kindIcon, listReports, noteMax, quickReplies, reopenReport, reportFilters, reportHref, searchBody, sortReports, stateCounts, titleLine, whoName, type ReportFilter, type ReportView } from './reports.ts';
	import { onMount, tick, untrack } from 'svelte';
	import { flip } from 'svelte/animate';
	import { fade, fly } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { replaceState } from '$app/navigation';
	import { page } from '$app/state';
	import { api, message } from '#lib/core/api.ts';
	import { to } from '#lib/core/paths.ts';
	import { proxied } from '#lib/core/tmdb-image.ts';
	import { live } from '#lib/core/live.svelte.ts';
	import { session } from '#lib/core/session.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { refreshInbox } from '../core/inbox.svelte.ts';
	import { ago, dateTime } from '#lib/core/format.ts';
	import type { SearchView } from '#lib/core/types.ts';

	// Reports, the Requests page's sibling: one toolbar (the tabs as a pill track, the way back to Requests), then window cards with the title's wide art, the problem's tag, the title and its facts, and a glass panel for answering.
	const manage = $derived(session.can('manage_library'));
	let reports = $state<ReportView[]>([]);
	let loaded = $state(false);
	let error = $state('');
	// Managers start on Open (what needs doing); everyone else on All, so a fixed report stays in sight.
	const start: ReportFilter = untrack(() => (session.can('manage_library') ? 'open' : 'all'));
	let filter = $state<ReportFilter>(page.url.searchParams.get('show') ? filterOf(page.url.searchParams.get('show')) : start);
	let busy = $state(0);
	// Just marked fixed: the card holds its Fixed beat on the Open tab before it leaves.
	let beat = $state(new Set<number>());

	// Reduced motion: Svelte transitions and the cards' reflow run in no time.
	const reduced = () => typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
	const ms = (d: number) => (reduced() ? 0 : d);

	const counts = $derived(stateCounts(reports));
	// During its beat a card counts as open still, so it holds its place under the Fixed mark.
	const held = $derived(reports.map((r) => (beat.has(r.id) ? { ...r, state: 'open' } : r)));
	const shown = $derived(sortReports(filter === 'all' ? held : held.filter((r) => r.state === filter)));

	async function load() {
		try {
			reports = (await listReports('all', { limit: 200 })).reports;
			error = '';
		} catch (e) {
			error = message(e);
		} finally {
			loaded = true;
		}
	}

	onMount(() => {
		void load();
		const offs = [
			live.on('event', (x) => {
				if (/^report_/.test((x as { Kind?: string })?.Kind ?? '')) void load();
			}),
			live.on('resync', () => void load())
		];
		void readSearches();
		return () => (offs.forEach((off) => off()), clearTimeout(searchTimer));
	});

	// Better copy: the card says Searching while that title's search runs (people who manage the Library read the searches list), then a toast gives that search's own outcome. Read once on opening, and again every few seconds only while a search runs.
	let searching = $state(new Set<number>());
	let searchTimer: ReturnType<typeof setTimeout> | undefined;
	// The searches started from this page: only those get an outcome toast.
	const mine = new Set<number>();
	async function readSearches() {
		clearTimeout(searchTimer);
		if (!session.can('manage_library')) return;
		const before = searching;
		let recent: SearchView[] = [];
		try {
			const r = await api.get<{ running: SearchView[] | null; recent?: SearchView[] | null }>('/searches');
			searching = new Set((r.running ?? []).filter((x) => x.title_id && x.kind !== 'feed_check' && (x.state === 'running' || x.state === 'queued')).map((x) => x.title_id as number));
			recent = r.recent ?? [];
		} catch {
			return; // Keep the last answer.
		}
		// Every search that ended in this read becomes one toast, however many there are.
		const ended: NonNullable<ReturnType<typeof searchEnded>>[] = [];
		for (const id of before) if (!searching.has(id)) {
			const e = searchEnded(id, recent);
			if (e) ended.push(e);
		}
		if (ended.length === 1) {
			const [e] = ended;
			if (e.kind === 'none') toasts.push(t('requests.reports.noBetter', { name: e.name }), 'info', { body: t('requests.reports.nothingYet') });
			else if (e.kind === 'failed') toasts.push(t('requests.reports.searchFailed', { name: e.name }), 'fail', e.error ? { body: e.error } : {});
			else toasts.push(t('requests.reports.anotherFound', { name: e.name }), 'ok');
		} else if (ended.length > 1) {
			const found = ended.filter((e) => e.kind === 'found').length;
			const failed = ended.filter((e) => e.kind === 'failed').length;
			const body = [found && t('requests.reports.nFound', { n: found }), failed && t('requests.reports.nFailed', { n: failed }), ended.length - found - failed && t('requests.reports.nNothingNew', { n: ended.length - found - failed })].filter(Boolean).join(', ');
			toasts.push(t('requests.reports.nFinished', { n: ended.length }), failed === ended.length ? 'fail' : found ? 'ok' : 'info', { body });
		}
		if (searching.size) searchTimer = setTimeout(() => void readSearches(), 2500);
	}
	function searchEnded(titleId: number, recent: SearchView[]): { name: string; kind: 'none' | 'failed' | 'found'; error?: string } | undefined {
		if (!mine.delete(titleId)) return;
		const job = recent.filter((x) => x.title_id === titleId).sort((x, y) => y.job_id - x.job_id)[0];
		const name = job?.title_name ?? reports.find((r) => r.title_id === titleId)?.title_name;
		if (!job || !name) return;
		if (job.outcome === 'none') return { name, kind: 'none' };
		if (job.outcome === 'failed') return { name, kind: 'failed', error: job.error };
		if (job.outcome === 'chosen' || job.outcome === 'held' || job.outcome === 'episodes' || job.outcome === 'album') return { name, kind: 'found' };
	}

	// The wide art comes from each title (a report carries only its poster). A title that gives none keeps the poster, blurred for a movie or show and as it is for an artist.
	let wides = $state<Record<number, string>>({});
	const asked = new Set<number>();
	$effect(() => {
		for (const r of shown.slice(0, 30)) {
			if (asked.has(r.title_id)) continue;
			asked.add(r.title_id);
			void api
				.get<{ backdrop?: string }>(`/titles/${r.title_id}`)
				.then((full) => {
					const b = (full.backdrop ?? '').replace('/t/p/w1280/', '/t/p/w780/').replace('/t/p/original/', '/t/p/w780/');
					if (!b) return;
					const p = proxied(b);
					wides[r.title_id] = p !== b ? p : to(b);
				})
				.catch(() => {});
		}
	});

	function pick(f: ReportFilter) {
		filter = f;
		const url = new URL(location.href);
		if (f === start) url.searchParams.delete('show');
		else url.searchParams.set('show', f);
		replaceState(url, page.state);
	}

	function put(r: ReportView) {
		reports = reports.map((x) => (x.id === r.id ? r : x));
		void refreshInbox();
	}

	const who = (r: ReportView) => (r.mine ? t('requests.reports.you') : whoName(r.made_by) || t('requests.reports.someone'));
	const face = (r: ReportView) => whoName(r.made_by) || session.s.name || t('requests.reports.you');
	const fixedBy = (r: ReportView) => (r.closed_by ? (r.closed_by.id === session.s.id ? t('requests.reports.byYou') : t('requests.reports.byName', { name: whoName(r.closed_by) })) : '');
	const poster = (r: ReportView) => (r.poster ? to(r.poster) : '');

	// The window: one report, read as a short conversation, and answered there.
	let openId = $state(0);
	let winOpen = $state(false);
	const win = $derived(reports.find((r) => r.id === openId) ?? null);
	let reply = $state('');
	let fixError = $state('');
	let replyBox: HTMLTextAreaElement | undefined = $state();
	function show(r: ReportView, answer = false) {
		openId = r.id;
		reply = '';
		fixError = '';
		winOpen = true;
		if (answer) void tick().then(() => replyBox?.focus());
	}
	// A quick reply adds its words to what is typed; tapping it again takes them out.
	function quick(q: string) {
		const has = reply.includes(q);
		const rest = reply.replace(q, '').replace(/\s{2,}/g, ' ').trim();
		reply = has ? rest : (rest ? `${rest} ${q}` : q).slice(0, noteMax);
	}

	async function fix() {
		const r = win;
		if (!r || busy) return;
		busy = r.id;
		fixError = '';
		try {
			const done = await fixReport(r.id, reply);
			beat = new Set(beat).add(r.id);
			put(done);
			winOpen = false;
			toasts.push(t('requests.reports.markedFixed'), 'ok', r.made_by && !r.mine ? { body: t('requests.reports.getsMessage', { name: whoName(r.made_by) }) } : {});
			setTimeout(() => {
				const next = new Set(beat);
				next.delete(r.id);
				beat = next;
			}, ms(1100));
		} catch (e) {
			fixError = message(e);
		} finally {
			busy = 0;
		}
	}
	async function reopen(r: ReportView) {
		if (busy) return;
		busy = r.id;
		try {
			put(await reopenReport(r.id));
		} catch (e) {
			toasts.push(t('requests.reports.reopenFailed'), 'fail', { body: message(e) });
		} finally {
			busy = 0;
		}
	}
	async function findBetter(r: ReportView) {
		if (busy) return;
		busy = r.id;
		try {
			await api.post(`/titles/${r.title_id}/search`, searchBody(r));
			// The card and the window say Searching from here on.
			searching = new Set(searching).add(r.title_id);
			mine.add(r.title_id);
			clearTimeout(searchTimer);
			searchTimer = setTimeout(() => void readSearches(), 1500);
		} catch (e) {
			toasts.push(t('requests.reports.startFailed'), 'fail', { body: message(e) });
		} finally {
			busy = 0;
		}
	}

	const calm = $derived(
		filter === 'fixed'
			? { title: t('requests.reports.calm.fixedTitle'), text: t('requests.reports.calm.fixedText') }
			: manage
				? { title: filter === 'open' ? t('requests.reports.calm.noOpen') : t('requests.reports.calm.none'), text: t('requests.reports.calm.managerText') }
				: { title: t('requests.reports.calm.none'), text: t('requests.reports.calm.memberText') }
	);
</script>

<div class="rp" data-testid="reports">
	<div class="toolbar">
		<div class="lead">
			<h1 class="rp-h">{manage ? t('requests.reports.title') : t('requests.reports.titleMine')}</h1>
			<div class="tabs" role="group" aria-label={t('requests.reports.show')}>
				{#each reportFilters as f (f.filter)}
					{@const n = counts[f.filter]}
					<div class="pw" class:on={filter === f.filter} role="presentation">
						<button type="button" class="tab" class:zero={!n} aria-pressed={filter === f.filter} onclick={() => pick(f.filter)}>{f.label}{#if loaded && !error}<span class="n" class:wait={f.filter === 'open' && n > 0}>{n}</span>{/if}</button>
					</div>
				{/each}
			</div>
		</div>
		{#if manage}<a class="back" href={to('/requests')}><Icon name="chevronLeft" size={16} /><span>{t('requests.reports.requests')}</span></a>{/if}
	</div>

	{#if error}
		<LoadError hasData={reports.length > 0} message={error} onretry={load} />
	{:else if !loaded}
		<ul class="cards" aria-hidden="true">{#each Array(3) as _, i (i)}<li class="bc ghost" style:--i={i}></li>{/each}</ul>
	{:else}
		<ul class="cards" aria-label={t('requests.reports.title')} in:fade={{ duration: ms(260) }}>
			{#each shown as r, i (r.id)}
				{@const wide = wides[r.title_id]}
				{@const open = r.state === 'open'}
				<li class="bc" class:answered={!open} data-testid="report" animate:flip={{ duration: ms(340), easing: cubicOut }} in:fly={{ y: 14, duration: ms(420), delay: ms(Math.min(i, 8) * 45), easing: cubicOut }} out:fade={{ duration: ms(220) }}>
					<!-- The whole card opens the report; the answers sit above this layer. -->
					<button type="button" class="open" aria-label={t('requests.reports.openReport', { title: titleLine(r) })} aria-haspopup="dialog" onclick={() => show(r)}></button>
					<a class="cl-go" href={to(reportHref(r))} aria-label={t('requests.reports.pageFor', { name: r.title_name })} title={t('requests.reports.openTitle')}><Icon name="arrowUpRight" size={16} /></a>
					<div class="ban" class:noart={!wide && !r.poster}>
						{#if wide}<img class="ban-img" src={wide} alt="" loading="lazy" decoding="async" />{:else if r.poster}<span class="ban-img" class:soft={r.title_kind !== 'artist'} aria-hidden="true" style:background-image={`url("${poster(r)}")`}></span>{/if}
					</div>
					<div class="top">
						<div class="ban-cap">
							<span class="tag"><Icon name={kindIcon(r.kind)} size={13} /><span data-testid="report-kind">{serverWords(r.kind_label)}</span></span>
							<h2 class="ban-t">{r.title_name}</h2>
							<span class="ban-f">{factsLine(r)}</span>
							<span class="vh" data-testid="report-state">{serverWords(r.state_label)}</span>
						</div>
					</div>
					<div class="sheet-b">
						<div class="body">
							<span class="who"><Avatar name={face(r)} size={26} /><span class="gl-id"><span class="gl-n"><b>{who(r)}</b></span><span class="gl-when"><span class="wide">{t('requests.reports.reported')}{' '}</span><time datetime={r.created_at} title={dateTime(r.created_at)}>{ago(r.created_at)}</time></span></span></span>
							<p class="note" class:none={!r.note}>{r.note || t('requests.reports.noNote')}</p>
							{#if !open && r.reply}<p class="re"><Icon name="check" size={13} /><span data-testid="report-reply">{r.reply}</span></p>{/if}
						</div>
						<div class="foot" class:one={!manage || !open} class:re-row={manage && !open}>
							{#if manage && open}
								{#if searching.has(r.title_id)}
									<span class="st-cap st-go" role="status" data-testid="report-searching" in:fade={{ duration: ms(200) }}><Icon name="search" size={15} /><span class="bl">{t('requests.reports.searching')}</span><span class="vh">{t('requests.reports.forBetter', { name: r.title_name })}</span></span>
								{:else}
									<button type="button" class="b no" aria-label={t('requests.reports.findBetter', { name: r.title_name })} disabled={busy === r.id} onclick={() => void findBetter(r)}><Icon name="search" size={15} /><span class="bl">{t('requests.reports.betterCopy')}</span></button>
								{/if}
								<button type="button" class="b yes" aria-label={t('requests.reports.markFixedName', { name: r.title_name })} onclick={() => show(r, true)}><Icon name="check" size={15} /><span class="bl">{t('requests.reports.markFixed')}</span></button>
							{:else}
								<span class="st-chip st-{open ? 'wait' : 'ok'}">
									<span class="st-sq"><Icon name={open ? 'clock' : 'check'} size={15} /></span>
									<span class="st-id"><b>{open ? t('requests.reports.open') : t('requests.reports.fixed')}</b><span class="st-d">{#if open}{t('requests.reports.waiting')}{:else}{#if fixedBy(r)}<span class="wide">{fixedBy(r)}{r.closed_at ? ' · ' : ''}</span>{/if}{#if r.closed_at}<time datetime={r.closed_at} title={dateTime(r.closed_at)}>{ago(r.closed_at)}</time>{/if}{/if}</span></span>
								</span>
								{#if manage}<button type="button" class="b no re-b" aria-label={t('requests.reports.reopenName', { name: r.title_name })} disabled={busy === r.id} onclick={() => void reopen(r)}><Icon name="refresh" size={15} /></button>{/if}
							{/if}
						</div>
					</div>
					{#if beat.has(r.id)}
						<span class="done" in:fade={{ duration: ms(180) }}><span class="done-ic" in:fly={{ y: 6, duration: ms(320), easing: cubicOut }}><Icon name="check" size={22} /></span>{t('requests.reports.fixed')}</span>
					{/if}
				</li>
			{:else}
				<li class="calm">
					<span class="calm-ic"><Icon name="check" size={24} /></span>
					<h2>{calm.title}</h2>
					<p>{calm.text}</p>
				</li>
			{/each}
		</ul>
	{/if}
</div>

{#snippet answers()}
	{#if win}
		<div class="rw-acts">
			{#if win.state === 'open'}
				<Button icon="search" busy={searching.has(win.title_id)} disabled={busy === win.id} onclick={() => win && void findBetter(win)}>{searching.has(win.title_id) ? t('requests.reports.searching') : t('requests.reports.betterCopy')}</Button>
				<Button variant="primary" icon="check" busy={busy === win.id} onclick={() => void fix()}>{t('requests.reports.markFixed')}</Button>
			{:else}
				<Button icon="refresh" busy={busy === win.id} onclick={() => win && void reopen(win)}>{t('requests.reports.reopen')}</Button>
			{/if}
		</div>
	{/if}
{/snippet}

<Sheet bind:open={winOpen} title={win ? titleLine(win) : t('requests.reports.report')} description={win ? factsLine(win) : undefined} size="md" placement="auto" footer={manage ? answers : undefined}>
	{#if win}
		{@const open = win.state === 'open'}
		{@const wide = wides[win.title_id]}
		<div class="rw" data-testid="report-window">
			<div class="rw-art" class:noart={!wide && !win.poster}>
				{#if wide}<img class="rw-img" src={wide} alt="" />{:else if win.poster}<span class="rw-img" class:soft={win.title_kind !== 'artist'} aria-hidden="true" style:background-image={`url("${poster(win)}")`}></span>{/if}
				<a class="rw-go" href={to(reportHref(win))} aria-label={t('requests.reports.openName', { name: win.title_name })} title={t('requests.reports.openTitle')}><Icon name="arrowUpRight" size={16} /></a>
				<span class="rw-state" class:ok={!open}><span class="dot"></span>{serverWords(win.state_label)}</span>
				<div class="rw-kind">
					<span class="rw-ic"><Icon name={kindIcon(win.kind)} size={20} /></span>
					<span class="rw-kt"><b>{serverWords(win.kind_label)}</b>{#if kindHint(win.kind)}<small>{kindHint(win.kind)}</small>{/if}</span>
				</div>
			</div>

			<div class="rw-msg">
				<Avatar name={face(win)} size={30} />
				<div class="rw-bub">
					<p class="rw-by"><b>{who(win)}</b><time datetime={win.created_at} title={dateTime(win.created_at)}>{ago(win.created_at)}</time></p>
					<p class="rw-text" class:none={!win.note}>{win.note || t('requests.reports.noNote')}</p>
				</div>
			</div>

			{#if !open}
				<div class="rw-msg back-msg" in:fly={{ y: 8, duration: ms(320), easing: cubicOut }}>
					<div class="rw-bub ok">
						<p class="rw-by"><b>{t('requests.reports.fixedBy', { by: fixedBy(win) })}</b>{#if win.closed_at}<time datetime={win.closed_at} title={dateTime(win.closed_at)}>{ago(win.closed_at)}</time>{/if}</p>
						{#if win.reply}<p class="rw-text">{win.reply}</p>{/if}
					</div>
					<span class="rw-okic"><Icon name="check" size={16} /></span>
				</div>
			{:else if manage}
				<form class="rw-form" onsubmit={(e) => { e.preventDefault(); void fix(); }}>
					<label class="rw-field">
						<span>{t('requests.reports.replyTo', { name: win.made_by && !win.mine ? whoName(win.made_by) : t('requests.reports.thePerson') })} <small>{t('requests.reportSheet.optional')}</small></span>
						<textarea bind:this={replyBox} bind:value={reply} rows="2" maxlength={noteMax} placeholder={t('requests.reports.replyPlaceholder')}></textarea>
					</label>
					<div class="rw-quick" role="group" aria-label={t('requests.reports.quickReplies')}>
						{#each quickReplies(win.kind) as q (q)}
							<button type="button" class="rw-q" class:on={reply.includes(q)} aria-pressed={reply.includes(q)} onclick={() => quick(q)}>{q}</button>
						{/each}
					</div>
					<p class="rw-hint">{reply.trim() ? t('requests.reports.hintReply') : t('requests.reports.hint')}</p>
					{#if fixError}<p class="rw-err" role="alert">{fixError}</p>{/if}
				</form>
			{/if}
		</div>
	{/if}
</Sheet>

<style>
	.rp { --ez: cubic-bezier(0.32, 0.72, 0, 1); display: flex; flex-direction: column; gap: 16px; min-width: 0; }
	button { font: inherit; color: inherit; }
	button:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.vh { position: absolute; width: 1px; height: 1px; margin: -1px; padding: 0; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; border: 0; }

	.toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
	.lead { display: flex; align-items: center; gap: 16px; min-width: 0; }
	.rp-h { margin: 0; font: 800 22px/1.05 var(--font-display, var(--font-ui)); letter-spacing: -0.02em; white-space: nowrap; }
	.tabs { position: relative; display: flex; align-items: center; gap: 2px; height: 40px; box-sizing: border-box; padding: 3px; border: 1px solid var(--line); border-radius: 12px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); backdrop-filter: blur(18px) saturate(1.4); -webkit-backdrop-filter: blur(18px) saturate(1.4); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 6px 20px -10px rgb(0 0 0 / 0.35); }
	.pw { position: relative; display: flex; align-items: center; flex: none; height: 32px; }
	.pw.on .tab { background: var(--seg-thumb); box-shadow: 0 0 0 1px var(--line-strong), 0 1px 2px rgb(0 0 0 / 0.12); }
	.tab { position: relative; z-index: 1; display: inline-flex; align-items: center; gap: 7px; height: 32px; padding: 0 13px; border: 0; border-radius: 9px; background: none; color: color-mix(in srgb, var(--text) 88%, var(--bg)); font: 500 13.5px/1 var(--font-ui); white-space: nowrap; cursor: pointer; transition: color 200ms var(--ease-out), background-color 200ms var(--ease-out), box-shadow 200ms var(--ease-out); }
	.tab:hover, .tab[aria-pressed='true'] { color: var(--text); }
	.tab:hover:not([aria-pressed='true']) { background: color-mix(in srgb, var(--text) 6%, transparent); }
	.tab.zero:not([aria-pressed='true']):not(:hover) { color: var(--text-muted); }
	.n { font-weight: 600; font-variant-numeric: tabular-nums; }
	.n.wait { color: var(--wait-text, var(--wait)); }
	:global(:root[data-theme='dark']) .pw.on .n.wait { color: #ffd08a; }
	@media (prefers-color-scheme: dark) { :global(:root:not([data-theme='light'])) .pw.on .n.wait { color: #ffd08a; } }
	.back { flex: none; display: inline-flex; align-items: center; gap: 5px; height: 40px; padding: 0 14px 0 10px; border: 1px solid var(--line); border-radius: 12px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); color: var(--text); font: 500 13px/1 var(--font-ui); white-space: nowrap; text-decoration: none; -webkit-backdrop-filter: blur(18px) saturate(1.4); backdrop-filter: blur(18px) saturate(1.4); transition: background 160ms var(--ease-out); }
	.back:hover { background: var(--surface-2); }
	.back:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.back :global(svg) { color: var(--text-muted); transition: transform 220ms var(--ez); }
	.back:hover :global(svg) { transform: translateX(-2px); }
	@media (pointer: coarse) { .tab::after { content: ''; position: absolute; inset: -6px 0; } .back { height: 44px; } }

	.cards { position: relative; display: grid; grid-template-columns: repeat(auto-fill, minmax(min(290px, 100%), 1fr)); gap: 16px; margin: 0; padding: 0; list-style: none; }
	.bc { --top: 170px; --gap: 10px; position: relative; isolation: isolate; overflow: hidden; display: flex; flex-direction: column; border-radius: 20px; background: #17181d; box-shadow: 0 0 0 1px var(--line), 0 12px 32px -16px rgb(0 0 0 / 0.6); }
	.bc.ghost { height: 318px; background: var(--surface-1); animation: ghost 1.4s ease-in-out calc(var(--i) * 140ms) infinite alternate; }
	@keyframes ghost { from { opacity: 0.9; } to { opacity: 0.45; } }
	.cl-go { position: absolute; top: 10px; right: 10px; z-index: 5; display: grid; place-items: center; width: 32px; height: 32px; border-radius: 10px; color: #fff; background: rgb(20 20 24 / 0.42); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.18), 0 0 0 1px rgb(255 255 255 / 0.14), 0 6px 16px -6px rgb(0 0 0 / 0.5); backdrop-filter: blur(14px) saturate(1.4); -webkit-backdrop-filter: blur(14px) saturate(1.4); opacity: 0; transform: translateY(-3px) scale(0.96); transition: opacity 280ms cubic-bezier(0.22, 1, 0.36, 1), transform 280ms cubic-bezier(0.22, 1, 0.36, 1), background 160ms ease; }
	.cl-go:hover { background: rgb(20 20 24 / 0.62); }
	.cl-go:focus-visible { opacity: 1; transform: none; outline: none; box-shadow: var(--focus-ring); }
	@media (hover: hover) and (pointer: fine) { .bc:hover .cl-go { opacity: 1; transform: none; } }
	@media (hover: none), (pointer: coarse) { .cl-go { display: none; } }
	.open { position: absolute; inset: 0; z-index: 1; padding: 0; border: 0; border-radius: inherit; background: none; cursor: pointer; }
	.open:focus-visible { box-shadow: none; }
	.bc:has(.open:focus-visible) { box-shadow: 0 0 0 2px var(--text), 0 24px 48px -24px rgb(0 0 0 / 0.6); }
	.ban { position: absolute; inset: 0; z-index: -1; overflow: hidden; background: #17181d; border-radius: inherit; }
	.ban.noart { background: radial-gradient(120% 90% at 15% 0%, #3b4263 0%, transparent 60%), radial-gradient(90% 80% at 100% 100%, #583a58 0%, transparent 58%), #17181d; }
	.ban-img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; object-position: center 28%; background-size: cover; background-position: center 28%; transform: scale(1.02); transition: transform 400ms var(--ez), filter 500ms var(--ease-out); }
	img.ban-img { animation: ban-in 600ms var(--ease-out) both; }
	@keyframes ban-in { from { opacity: 0; } }
	.ban-img.soft { filter: blur(18px) saturate(1.3); transform: scale(1.3); opacity: 0.8; }
	.ban::after { content: ''; position: absolute; inset: 0; background: linear-gradient(to bottom, rgb(0 0 0 / 0.22), transparent calc(var(--top) * 0.22), rgb(0 0 0 / 0.6) calc(var(--top) * 0.6), rgb(0 0 0 / 0.8) var(--top), rgb(0 0 0 / 0.4)); }
	.answered .ban-img:not(.soft) { filter: saturate(0.6); }
	.top { flex: none; height: var(--top); display: flex; align-items: flex-end; box-sizing: border-box; padding: 0 14px 10px; }
	.ban-cap { min-width: 0; display: flex; flex-direction: column; gap: 3px; color: #fff; }
	.tag { align-self: flex-start; display: inline-flex; align-items: center; gap: 5px; height: 24px; margin-bottom: 5px; padding: 0 10px 0 8px; border-radius: 999px; background: rgb(20 20 24 / 0.5); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.2), 0 0 0 1px rgb(255 255 255 / 0.16); color: #fff; font: 600 12px/1 var(--font-ui); -webkit-backdrop-filter: blur(14px) saturate(1.4); backdrop-filter: blur(14px) saturate(1.4); }
	.ban-t { margin: 0; font: 800 22px/1.05 var(--font-display, var(--font-ui)); letter-spacing: -0.02em; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
	.ban-f { font: 600 12px/1.3 var(--font-ui); color: rgb(255 255 255 / 0.85); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	/* Clicks pass through the panel to the card's open layer; only its buttons take their own. */
	.sheet-b, .top { pointer-events: none; }
	.sheet-b button { pointer-events: auto; }
	/* The glass panel redefines --text, --text-muted and --bg so the ink reads on the blurred art. */
	.sheet-b { --blur: blur(22px) saturate(1.3); --text: light-dark(#14161c, #f5f5f7); --text-muted: light-dark(#1c2029, rgb(235 235 245 / 0.8)); --bg: light-dark(#eef0f5, #17171b); --g1: light-dark(rgb(255 255 255 / 0.72), rgb(34 34 40 / 0.7)); --g2: light-dark(rgb(246 248 252 / 0.7), rgb(10 10 13 / 0.8)); --g3: light-dark(rgb(255 255 255 / 0.06), transparent); --edge: inset 0 1px 0 light-dark(rgb(255 255 255 / 0.75), rgb(255 255 255 / 0.16)), inset 0 0 0 1px light-dark(rgb(30 36 58 / 0.07), rgb(255 255 255 / 0.05)); --st-h: 40px; --st-wait: light-dark(#8a5a00, #ffb340); --st-wait-ink: light-dark(#5c3b00, #ffd699); --st-ok: light-dark(#18843f, #34c26a); --st-ok-ink: light-dark(#073d1b, #a6efc0); --st-go: light-dark(#1d5fd6, #4b8dff); --st-go-ink: light-dark(#072a6b, #b5d1ff); position: relative; z-index: 1; display: flex; flex-direction: column; flex: 1; margin: 0 var(--gap) var(--gap); border-radius: calc(20px - var(--gap)); background: linear-gradient(180deg, var(--g1), var(--g2)), linear-gradient(var(--g3), var(--g3)); color: var(--text); -webkit-backdrop-filter: var(--blur); backdrop-filter: var(--blur); box-shadow: var(--edge); }
	:global(:root[data-theme='light']) .sheet-b { --blur: blur(30px) saturate(1.3) brightness(1.14); }
	@media (prefers-color-scheme: light) { :global(:root:not([data-theme])) .sheet-b { --blur: blur(30px) saturate(1.3) brightness(1.14); } }
	.body { display: flex; flex-direction: column; gap: 8px; padding: 12px 12px 0; min-width: 0; }
	.who { display: flex; align-items: center; gap: 9px; min-width: 0; color: var(--text-muted); font: var(--t-footnote); white-space: nowrap; }
	.who > :global(*) { flex: none; }
	.gl-id { display: flex; flex: 1 1 auto; flex-direction: column; gap: 1px; min-width: 0; }
	.gl-n { display: flex; align-items: center; gap: 6px; min-width: 0; color: var(--text); font: 600 13.5px/1.3 var(--font-ui); }
	.gl-n > b { flex: 0 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.gl-when { overflow: hidden; color: var(--text-muted); font: 500 12px/1.3 var(--font-ui); text-overflow: ellipsis; white-space: nowrap; }
	/* Two lines on the card (kept two lines tall so a row of cards lines up), all of it in the window. */
	.note { margin: 0; min-height: calc(2 * 1.38em); color: var(--text); font: 500 13px/1.38 var(--font-ui); overflow-wrap: anywhere; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
	.note.none, .rw-text.none { color: var(--text-muted); font-style: italic; }
	.re { display: flex; align-items: flex-start; gap: 6px; margin: 0; color: var(--st-ok-ink); font: 500 12.5px/1.35 var(--font-ui); }
	.re :global(svg) { flex: none; margin-top: 2px; }
	.re > span { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.foot { position: relative; z-index: 2; display: grid; grid-template-columns: 1fr 1fr; gap: 8px; margin-top: auto; padding: 12px; }
	.foot.one { grid-template-columns: minmax(0, 1fr); }
	.foot.re-row { grid-template-columns: minmax(0, 1fr) auto; }
	.b { display: inline-flex; align-items: center; justify-content: center; gap: 6px; height: 40px; padding: 0 12px; border: 0; border-radius: 12px; font: var(--t-subhead); font-weight: 600; white-space: nowrap; cursor: pointer; transition: transform 140ms var(--ease-out), background 200ms var(--ease-out), opacity 200ms var(--ease-out); }
	.b:active { transform: scale(0.96); }
	.b:disabled { opacity: 0.55; cursor: default; }
	.b.no { background: color-mix(in srgb, var(--text) 8%, transparent); color: var(--text); box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--text) 10%, transparent); }
	.b.no:hover:not(:disabled) { background: color-mix(in srgb, var(--text) 13%, transparent); }
	.b.yes { background: var(--text); color: var(--bg); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.2), 0 6px 14px -8px rgb(0 0 0 / 0.6); }
	.b:focus-visible { box-shadow: var(--focus-ring); }
	.re-b { width: 40px; padding: 0; }
	.st-wait { --st-c: var(--st-wait); --st-ink: var(--st-wait-ink); }
	.st-ok { --st-c: var(--st-ok); --st-ink: var(--st-ok-ink); }
	.st-go { --st-c: var(--st-go); --st-ink: var(--st-go-ink); }
	.st-cap { position: relative; overflow: hidden; display: flex; align-items: center; justify-content: center; gap: 7px; min-width: 0; height: var(--st-h); padding: 0 12px; border-radius: 12px; background: light-dark(color-mix(in srgb, var(--st-c) 14%, rgb(255 255 255 / 0.5)), color-mix(in srgb, var(--st-c) 22%, transparent)); box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--st-c) 34%, transparent); color: var(--st-ink); font: var(--t-subhead); font-weight: 600; white-space: nowrap; }
	.st-cap::after { content: ''; position: absolute; inset: 0; background: linear-gradient(90deg, transparent, rgb(255 255 255 / 0.22), transparent) no-repeat; background-size: 40% 100%; animation: st-shim 2.2s ease-in-out infinite; }
	@keyframes st-shim { from { background-position: -60% 0; } to { background-position: 160% 0; } }
	.st-chip { display: flex; align-items: center; gap: 9px; min-width: 0; height: var(--st-h); }
	.st-sq { display: grid; place-items: center; flex: none; width: 26px; height: 26px; border-radius: 8px; background: linear-gradient(180deg, rgb(255 255 255 / 0.1), transparent 60%), color-mix(in srgb, var(--st-c) 20%, transparent); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.14), inset 0 0 0 1px color-mix(in srgb, var(--st-c) 42%, transparent); color: var(--st-ink); }
	.st-id { display: flex; flex-direction: column; gap: 1px; min-width: 0; }
	.st-id > b { color: var(--st-ink); font: 700 13.5px/1.3 var(--font-ui); }
	.st-id > .st-d { overflow: hidden; color: var(--text-muted); font: 500 12px/1.3 var(--font-ui); text-overflow: ellipsis; white-space: nowrap; }
	.done { position: absolute; inset: 0; z-index: 6; display: flex; align-items: center; justify-content: center; gap: 10px; background: color-mix(in srgb, var(--surface-1) 78%, transparent); -webkit-backdrop-filter: blur(6px); backdrop-filter: blur(6px); color: var(--text); font: var(--t-headline); }
	.done-ic { display: grid; place-items: center; width: 40px; height: 40px; border-radius: 50%; background: var(--ok); color: #fff; box-shadow: 0 8px 20px -8px color-mix(in srgb, var(--ok) 80%, transparent); }
	.calm { grid-column: 1 / -1; display: grid; justify-items: center; gap: 6px; padding: 64px 20px; border-radius: 18px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); color: var(--text-muted); text-align: center; }
	.calm-ic { display: grid; place-items: center; width: 52px; height: 52px; margin-bottom: 4px; border-radius: 50%; background: var(--surface-2); color: var(--text); }
	.calm h2 { margin: 0; color: var(--text); font: var(--t-title3); }
	.calm p { margin: 0; max-width: 46ch; }

	.ban::before { content: ''; position: absolute; inset: 0; z-index: 1; pointer-events: none; opacity: 0; background: linear-gradient(115deg, transparent 35%, rgb(255 255 255 / 0.22) 47%, rgb(255 250 235 / 0.34) 50%, rgb(255 255 255 / 0.22) 53%, transparent 65%) no-repeat; background-size: 250% 100%; background-position: 130% 0; transition: opacity 350ms ease-out; }
	@media (hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference) {
		.bc:hover .ban-img:not(.soft) { transform: scale(1.0608); transition: transform 5s cubic-bezier(0.42, 0, 0.3, 1), filter 500ms var(--ease-out); }
		.bc:hover .ban::before { opacity: 1; animation: fx-sweep 2.4s cubic-bezier(0.4, 0, 0.2, 1) both; }
	}
	@keyframes fx-sweep { from { background-position: 130% 0; } to { background-position: -30% 0; } }

	.rw { display: grid; gap: 14px; min-width: 0; }
	.rw-art { position: relative; isolation: isolate; overflow: hidden; display: flex; align-items: flex-end; height: 148px; padding: 14px; border-radius: 16px; background: #17181d; box-shadow: 0 0 0 1px var(--line); }
	.rw-art.noart { background: radial-gradient(120% 90% at 15% 0%, #3b4263 0%, transparent 60%), radial-gradient(90% 80% at 100% 100%, #583a58 0%, transparent 58%), #17181d; }
	.rw-img { position: absolute; inset: 0; z-index: -2; width: 100%; height: 100%; object-fit: cover; object-position: center 28%; background-size: cover; background-position: center 28%; }
	.rw-img.soft { filter: blur(18px) saturate(1.3); transform: scale(1.3); opacity: 0.8; }
	.rw-art::after { content: ''; position: absolute; inset: 0; z-index: -1; background: linear-gradient(to bottom, rgb(0 0 0 / 0.15), rgb(0 0 0 / 0.2) 35%, rgb(0 0 0 / 0.78)); }
	.rw-state { position: absolute; top: 12px; right: 12px; display: inline-flex; align-items: center; gap: 6px; height: 26px; padding: 0 11px 0 9px; border-radius: 999px; background: rgb(20 20 24 / 0.5); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.2), 0 0 0 1px rgb(255 255 255 / 0.16); color: #fff; font: 600 12px/1 var(--font-ui); -webkit-backdrop-filter: blur(14px) saturate(1.4); backdrop-filter: blur(14px) saturate(1.4); }
	.rw-state .dot { width: 7px; height: 7px; border-radius: 50%; background: var(--wait, #ffb340); }
	.rw-state.ok .dot { background: var(--ok, #30d158); }
	.rw-kind { display: flex; align-items: center; gap: 11px; min-width: 0; color: #fff; }
	.rw-ic { flex: none; display: grid; place-items: center; width: 42px; height: 42px; border-radius: 13px; background: rgb(255 255 255 / 0.16); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.28), 0 0 0 1px rgb(255 255 255 / 0.14); -webkit-backdrop-filter: blur(14px) saturate(1.4); backdrop-filter: blur(14px) saturate(1.4); }
	.rw-kt { display: grid; gap: 2px; min-width: 0; }
	.rw-kt b { font: 800 22px/1.05 var(--font-display, var(--font-ui)); letter-spacing: -0.02em; }
	.rw-kt small { color: rgb(255 255 255 / 0.85); font: 500 12.5px/1.3 var(--font-ui); }
	.rw-msg { display: flex; align-items: flex-end; gap: 9px; min-width: 0; }
	.rw-msg > :global(*) { flex: none; }
	.rw-msg.back-msg { justify-content: flex-end; }
	.rw-bub { flex: 0 1 auto !important; min-width: 0; max-width: 84%; padding: 10px 13px 11px; border-radius: 16px 16px 16px 5px; background: var(--surface-2); box-shadow: inset 0 0 0 1px var(--line); }
	.rw-bub.ok { border-radius: 16px 16px 5px 16px; background: color-mix(in srgb, var(--ok) 13%, var(--surface-1)); box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--ok) 32%, transparent); }
	.rw-by { display: flex; flex-wrap: wrap; align-items: baseline; gap: 2px 8px; margin: 0; font: 600 13px/1.3 var(--font-ui); }
	.rw-by time { color: var(--text-muted); font: 500 12px/1.3 var(--font-ui); }
	.rw-text { margin: 4px 0 0; font: 500 14.5px/1.42 var(--font-ui); white-space: pre-wrap; overflow-wrap: anywhere; }
	.rw-okic { display: grid; place-items: center; width: 30px; height: 30px; border-radius: 50%; background: var(--ok); color: #fff; }
	.rw-form { display: grid; gap: 9px; padding-top: 14px; border-top: 1px solid var(--line); }
	.rw-field { display: grid; gap: 6px; }
	.rw-field > span { font: 600 14px/1.3 var(--font-ui); }
	.rw-field small { color: var(--text-muted); font-weight: 500; }
	.rw-field textarea { width: 100%; min-width: 0; min-height: 62px; padding: 10px 12px; border: 1px solid color-mix(in srgb, var(--text) 14%, transparent); border-radius: 12px; background: var(--surface-1, var(--bg)); color: var(--text); font: 500 14px/1.4 var(--font-ui); resize: vertical; transition: border-color 160ms var(--ease-out); }
	.rw-field textarea:focus-visible { outline: none; border-color: color-mix(in srgb, var(--text) 45%, transparent); }
	.rw-quick { display: flex; flex-wrap: wrap; gap: 6px; }
	.rw-q { min-height: 32px; padding: 0 12px; border: 0; border-radius: 999px; background: var(--surface-2); box-shadow: inset 0 0 0 1px var(--line); color: var(--text); font: 500 13px/1 var(--font-ui); cursor: pointer; transition: transform 140ms var(--ease-out), background 180ms var(--ease-out), color 180ms var(--ease-out); }
	.rw-q:hover { background: var(--surface-3); }
	.rw-q:active { transform: scale(0.96); }
	.rw-q.on { background: var(--text); box-shadow: none; color: var(--bg); }
	.rw-hint { margin: 0; color: var(--text-muted); font: var(--t-footnote); }
	.rw-err { margin: 0; color: var(--fail-text, #e5484d); font: 500 14px/1.4 var(--font-ui); }
	.rw-go { position: absolute; top: 12px; left: 12px; display: grid; place-items: center; width: 32px; height: 32px; border-radius: 10px; background: rgb(20 20 24 / 0.5); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.2), 0 0 0 1px rgb(255 255 255 / 0.16); color: #fff; -webkit-backdrop-filter: blur(14px) saturate(1.4); backdrop-filter: blur(14px) saturate(1.4); transition: background 160ms var(--ease-out), transform 220ms var(--ez); }
	.rw-go:hover { background: rgb(20 20 24 / 0.7); transform: translate(1px, -1px); }
	.rw-go:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.rw-acts { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 8px; }
	@media (pointer: coarse) { .b, .sheet-b { --st-h: 44px; } .b { height: 44px; } .re-b { width: 44px; } .rw-q { min-height: 40px; } .rw-go { width: 44px; height: 44px; border-radius: 13px; } }

	@media (max-width: 767px) {
		.rp { gap: 12px; padding-bottom: calc(var(--tabbar, 64px) + 16px + env(safe-area-inset-bottom, 0px)); }
		.toolbar { align-items: flex-start; }
		.lead { flex: 1; flex-direction: column; align-items: stretch; gap: 10px; }
		.rp-h { display: flex; align-items: center; height: 44px; font-size: 24px; }
		.toolbar { position: relative; }
		.back { position: absolute; top: 0; right: 0; }
		.tabs { width: 100%; }
		.pw { flex: 1; }
		.tab { flex: 1; justify-content: center; padding: 0 10px; }
		.cards { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
		.bc { --top: 124px; --gap: 8px; }
		.bc.ghost { height: 268px; }
		.top { padding: 0 10px 8px; }
		.tag { height: 22px; padding: 0 8px 0 7px; font-size: 11.5px; }
		.ban-t { font-size: 17px; }
		.ban-f { white-space: normal; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; }
		.sheet-b { border-radius: 12px; }
		.body { gap: 7px; padding: 10px 10px 0; }
		.who { gap: 7px; }
		.gl-n { font-size: 12.5px; }
		.gl-when { white-space: normal; text-overflow: clip; }
		.note { font-size: 12.5px; min-height: calc(3 * 1.38em); -webkit-line-clamp: 3; line-clamp: 3; }
		.foot { gap: 6px; padding: 10px; }
		/* Better copy and Mark fixed: equal halves, icons only (each has its full name for screen readers). */
		.b { padding: 0 8px; }
		.b .bl, .st-cap .bl { display: none; }
		.st-cap { padding: 0 8px; }
		.st-chip { gap: 7px; }
		.st-id > b { font-size: 12.5px; }
		.rw-art { height: 128px; }
		.wide { display: none; }
		.rw-acts { width: 100%; flex-wrap: nowrap; }
		.rw-acts > :global(*) { flex: 1; }
		.rw-bub { max-width: 88%; }
	}
	@media (max-width: 767px) and (pointer: coarse) { .tabs { height: 46px; padding-block: 6px; border-radius: 15px; } }
	@media (prefers-reduced-motion: reduce) {
		.bc.ghost, img.ban-img, .st-cap::after { animation: none; }
		.ban-img, .back :global(svg), .b, .rw-q, .rw-go { transition: none; }
		.b:active, .rw-q:active { transform: none; }
	}
</style>
