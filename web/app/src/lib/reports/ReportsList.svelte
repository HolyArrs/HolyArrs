<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { replaceState } from '$app/navigation';
	import { page } from '$app/state';
	import { api, message } from '#lib/api.ts';
	import { to } from '#lib/paths.ts';
	import { live } from '#lib/live.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { toasts } from '#lib/toasts.svelte.ts';
	import { refreshInbox } from '#lib/requests.svelte.ts';
	import { ago, dateTime } from '#lib/format.ts';
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Chip from '#lib/ui/Chip.svelte';
	import Menu, { type MenuItem } from '#lib/ui/Menu.svelte';
	import StatusPill from '#lib/ui/StatusPill.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import LoadError from '#lib/ui/LoadError.svelte';
	import Skeleton from '#lib/ui/Skeleton.svelte';
	import Icon from '#lib/Icon.svelte';
	import { filterOf, fixReport, listReports, noteMax, partLine, reopenReport, reportFilters, reportHref, searchBody, sortReports, stateCounts, titleLine, whoName, type ReportFilter, type ReportView } from '#lib/reports.ts';

	// Reported problems (#95): the owner and admins see every report, mark
	// one fixed (with an optional reply to the person) or open it again, and
	// jump to the title or look for a better copy. Everyone else sees only
	// their own. One column of cards, so phones never scroll sideways.
	const manage = $derived(session.can('manage_library'));
	let reports = $state<ReportView[]>([]);
	let loaded = $state(false);
	let error = $state('');
	// Managers start on Open (what needs doing); everyone else on All, so a fixed report stays in sight.
	const start: ReportFilter = untrack(() => (session.can('manage_library') ? 'open' : 'all'));
	let filter = $state<ReportFilter>(page.url.searchParams.get('show') ? filterOf(page.url.searchParams.get('show')) : start);
	let busy = $state(0);

	const counts = $derived(stateCounts(reports));
	const shown = $derived(sortReports(filter === 'all' ? reports : reports.filter((r) => r.state === filter)));

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
		return () => offs.forEach((off) => off());
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

	// Mark fixed: an optional reply goes to the person who reported it.
	let fixing = $state<ReportView | null>(null);
	let fixOpen = $state(false);
	let reply = $state('');
	let fixError = $state('');
	function askFix(r: ReportView) {
		fixing = r;
		reply = '';
		fixError = '';
		fixOpen = true;
	}
	async function fix() {
		const r = fixing;
		if (!r) return;
		busy = r.id;
		fixError = '';
		try {
			put(await fixReport(r.id, reply));
			fixOpen = false;
			toasts.push(r.made_by && !r.mine ? `Marked fixed. ${whoName(r.made_by)} gets a message.` : 'Marked fixed.', 'ok');
		} catch (e) {
			fixError = message(e);
		} finally {
			busy = 0;
		}
	}
	async function reopen(r: ReportView) {
		busy = r.id;
		try {
			put(await reopenReport(r.id));
			toasts.push('Opened again.', 'ok');
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = 0;
		}
	}
	async function findBetter(r: ReportView) {
		busy = r.id;
		try {
			await api.post(`/titles/${r.title_id}/search`, searchBody(r));
			toasts.push(`Looking for a better copy of ${r.title_name}.`, 'ok');
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = 0;
		}
	}
	function menuOf(r: ReportView): MenuItem[] {
		return [
			{ label: 'Open the title', icon: 'arrowUpRight', href: to(reportHref(r)) },
			{ label: 'Find a better copy', icon: 'search', onselect: () => void findBetter(r), disabled: busy === r.id }
		];
	}
</script>

<div class="rl" data-testid="reports">
	<div class="rl-bar">
		<div class="rl-chips" role="group" aria-label="Show">
			{#each reportFilters as f (f.filter)}
				<Chip label={f.label} count={counts[f.filter] || undefined} selected={filter === f.filter} onclick={() => pick(f.filter)} />
			{/each}
		</div>
		{#if loaded && !error}<p class="rl-count">{shown.length} {shown.length === 1 ? 'report' : 'reports'}</p>{/if}
	</div>

	{#if error}
		<LoadError hasData={reports.length > 0} message={error} onretry={load} />
	{:else if !loaded}
		<ul class="rl-list" aria-hidden="true">{#each Array(3) as _, i (i)}<li class="rl-card"><Skeleton lines={3} /></li>{/each}</ul>
	{:else if !shown.length}
		<EmptyState
			icon={filter === 'fixed' ? 'check' : 'alert'}
			title={filter === 'fixed' ? 'Nothing fixed yet' : filter === 'open' ? 'No open problems' : 'No reports yet'}
			text={manage
				? 'When someone reports a problem with a movie, show or album, it shows up here.'
				: 'Found something wrong with a title in the Library? Open it, press More and choose Report a problem.'}
		/>
	{:else}
		<ul class="rl-list">
			{#each shown as r (r.id)}
				{@const part = partLine(r)}
				<li class="rl-card" class:fixed={r.state === 'fixed'} data-testid="report">
					<a class="rl-art" href={to(reportHref(r))} tabindex="-1" aria-hidden="true">
						{#if r.poster}<img src={r.poster} alt="" loading="lazy" decoding="async" />{:else}<Icon name={r.title_kind === 'artist' ? 'music' : r.title_kind === 'show' ? 'tv' : 'film'} size={20} />{/if}
					</a>
					<div class="rl-main">
						<div class="rl-top">
							<div class="rl-names">
								<a class="rl-title" href={to(reportHref(r))}>{titleLine(r)}</a>
								{#if part}<span class="rl-part">{part}</span>{/if}
							</div>
							<StatusPill kind={r.state === 'open' ? 'missing' : 'in_library'} label={r.state_label} testid="report-state" />
						</div>
						<p class="rl-kind"><span class="rl-tag" data-testid="report-kind">{r.kind_label}</span>{#if r.note}<span class="rl-note">{r.note}</span>{/if}</p>
						<p class="rl-meta">
							{#if manage && r.made_by}{r.mine ? 'You' : whoName(r.made_by)} reported it{:else}Reported{/if}
							<time datetime={r.created_at} title={dateTime(r.created_at)}>{ago(r.created_at)}</time>
							{#if r.state === 'fixed' && r.closed_at}· fixed {#if r.closed_by}by {whoName(r.closed_by)} {/if}<time datetime={r.closed_at} title={dateTime(r.closed_at)}>{ago(r.closed_at)}</time>{/if}
						</p>
						{#if r.state === 'fixed' && r.reply}<p class="rl-reply" data-testid="report-reply"><Icon name="check" size={14} /><span>{r.reply}</span></p>{/if}
						{#if manage}
							<div class="rl-acts">
								{#if r.state === 'open'}
									<Button size="sm" variant="primary" icon="check" busy={busy === r.id} onclick={() => askFix(r)}>Mark fixed</Button>
								{:else}
									<Button size="sm" icon="refresh" busy={busy === r.id} onclick={() => void reopen(r)}>Reopen</Button>
								{/if}
								<Menu label="More for {r.title_name}" items={menuOf(r)} width={230} />
							</div>
						{/if}
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</div>

<Sheet bind:open={fixOpen} title="Mark fixed" description={fixing ? `${titleLine(fixing)}${partLine(fixing) ? `, ${partLine(fixing)}` : ''}: ${fixing.kind_label}` : undefined} size="sm" placement="auto">
	<form class="rl-fix" onsubmit={(e) => { e.preventDefault(); void fix(); }}>
		<label class="rl-field">
			<span>Reply to {fixing?.made_by && !fixing.mine ? whoName(fixing.made_by) : 'the person who reported it'} <small>(optional)</small></span>
			<textarea bind:value={reply} rows="3" maxlength={noteMax} placeholder="For example: swapped it for a better copy, try again tonight"></textarea>
		</label>
		<p class="rl-hint">They get a message that it is fixed{reply.trim() ? ', with your reply' : ''}.</p>
		{#if fixError}<p class="rl-err" role="alert">{fixError}</p>{/if}
	</form>
	{#snippet footer()}
		<Button variant="plain" onclick={() => (fixOpen = false)}>Cancel</Button>
		<Button variant="primary" icon="check" busy={!!fixing && busy === fixing.id} onclick={() => void fix()}>Mark fixed</Button>
	{/snippet}
</Sheet>

<style>
	.rl { display: grid; gap: 14px; min-width: 0; }
	.rl-bar { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px; }
	.rl-chips { display: flex; flex-wrap: wrap; gap: 8px; }
	.rl-count { margin: 0; color: var(--text-muted); font: var(--t-subhead); font-variant-numeric: tabular-nums; }
	.rl-list { display: grid; gap: 10px; margin: 0; padding: 0; list-style: none; }
	.rl-card { display: flex; gap: 14px; min-width: 0; padding: 14px; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: var(--shadow-card); }
	.rl-card.fixed .rl-art { opacity: .7; }
	.rl-art { flex: none; display: grid; place-items: center; width: 52px; height: 78px; overflow: hidden; border-radius: 8px; background: var(--surface-2); color: var(--text-muted); }
	.rl-art img { width: 100%; height: 100%; object-fit: cover; }
	.rl-main { flex: 1; min-width: 0; display: grid; gap: 6px; align-content: start; }
	.rl-top { display: flex; align-items: flex-start; justify-content: space-between; gap: 10px; min-width: 0; }
	.rl-names { display: grid; gap: 2px; min-width: 0; }
	.rl-title { color: var(--text); font: var(--t-headline); overflow-wrap: anywhere; }
	.rl-title:hover { text-decoration: underline; }
	.rl-part { color: var(--text-muted); font: var(--t-subhead); overflow-wrap: anywhere; }
	.rl-kind { display: flex; flex-wrap: wrap; align-items: baseline; gap: 6px 10px; margin: 0; font: var(--t-callout); }
	.rl-tag { display: inline-flex; align-items: center; height: 24px; padding: 0 10px; border-radius: 999px; background: var(--surface-2); font: 600 13px/1 var(--font-ui); }
	.rl-note { min-width: 0; overflow-wrap: anywhere; }
	.rl-meta { margin: 0; color: var(--text-muted); font: var(--t-footnote); }
	.rl-reply { display: flex; gap: 6px; align-items: flex-start; margin: 0; padding: 8px 10px; border-radius: 10px; background: var(--surface-2); font: var(--t-footnote); overflow-wrap: anywhere; }
	.rl-reply :global(svg) { flex: none; margin-top: 2px; color: var(--ok-text, var(--text-muted)); }
	.rl-acts { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin-top: 4px; }
	.rl-fix { display: grid; gap: 10px; }
	.rl-field { display: grid; gap: 6px; }
	.rl-field > span { font: 600 14px/1.3 var(--font-ui); }
	.rl-field small { color: var(--text-muted); font-weight: 500; }
	.rl-field textarea { width: 100%; min-width: 0; min-height: 76px; padding: 10px 12px; border: 1px solid color-mix(in srgb, var(--text) 14%, transparent); border-radius: 10px; background: var(--surface-1, var(--bg)); color: var(--text); font: 500 14px/1.4 var(--font-ui); resize: vertical; }
	.rl-hint { margin: 0; color: var(--text-muted); font: var(--t-footnote); }
	.rl-err { margin: 0; color: var(--fail-text, #e5484d); font: 500 14px/1.4 var(--font-ui); }
	@media (max-width: 480px) {
		.rl-card { gap: 12px; padding: 12px; }
		.rl-art { width: 44px; height: 66px; }
		.rl-top { flex-direction: column; gap: 6px; }
	}
</style>
