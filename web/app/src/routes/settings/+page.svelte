<script lang="ts">
	import { to } from '#lib/paths.ts';
	import Icon from '#lib/Icon.svelte';
	import { grouped, groups as allGroups, sections as all, visible } from '#lib/settings.ts';
	import { searchSettings, settingIndex, type SettingEntry } from '#lib/settings-search.ts';
	import { session } from '#lib/session.svelte.ts';
	import { partsNow } from '#lib/parts.svelte.ts';
	import { offPartName } from '#lib/parts.ts';
	import { settingsNow } from '#lib/settings-status.svelte.ts';
	import { groupState, pageState, stateWords, type GroupState } from '#lib/settings-status.ts';
	import PageHeader from '#lib/ui/PageHeader.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';

	const sections = $derived(visible(all, session.can).filter((section) => !offPartName(partsNow.answer, section.href)));
	const groups = $derived(grouped(sections));
	const open = $derived(new Set(sections.map((s) => s.href)));
	const sectionOf = (href: string) => all.find((s) => s.href === href);
	const groupLabel = (href: string) => allGroups.find((g) => g.id === sectionOf(href)?.group)?.label ?? '';

	// Pages are results too, so "subtitles" finds the page as well as its fields.
	const index = $derived<SettingEntry[]>([...sections.map((s) => ({ label: s.label, href: s.href, anchor: '', words: s.about })), ...settingIndex]);

	let query = $state('');
	const results = $derived(searchSettings(query, (h) => open.has(h), groupLabel, index).slice(0, 30));
	const answer = $derived(settingsNow.answer);
	const attention = $derived((answer?.items ?? []).filter((a) => open.has(a.href)));

	const tone = (s: GroupState) => (s === 'attention' ? 'fail' : s === 'unset' ? 'wait' : s === 'ok' ? 'ok' : '');
	const link = (r: SettingEntry) => to(r.href) + (r.anchor ? `#${r.anchor}` : '');
</script>

<svelte:head><title>Settings · HolyArrs</title></svelte:head>

<PageHeader title="Settings" subtitle="Everything HolyArrs can be set to, in seven groups." />

<div class="find">
	<Icon name="search" size={17} />
	<label class="sr-only" for="settings-find">Search settings</label>
	<input id="settings-find" class="input" type="search" placeholder="Search settings, like free space or password" autocomplete="off" bind:value={query} aria-controls="settings-results" />
</div>

{#if query.trim()}
	{#if results.length}
		<ul class="results" id="settings-results" aria-label="Matching settings" data-testid="setting-results">
			{#each results as r (r.href + '#' + r.anchor)}
				{@const s = sectionOf(r.href)}
				<li>
					<a class="row-link" href={link(r)} data-testid="setting-result">
						<span class="setting-icon"><Icon name={s?.icon ?? 'settings'} size={17} /></span>
						<span class="setting-copy">
							<strong>{r.label}</strong>
							<span>{r.anchor ? `${groupLabel(r.href)} › ${s?.label ?? ''}${r.advanced ? ' › Advanced' : ''}` : `${groupLabel(r.href)} · page`}</span>
						</span>
						<Icon name="chevronRight" size={16} />
					</a>
				</li>
			{/each}
		</ul>
	{:else}
		<EmptyState compact icon="search" title="No settings match" text="Try another word, like folder, key or password." action={{ label: 'Clear the search', onclick: () => (query = '') }} />
	{/if}
{:else}
	{#if attention.length}
		<section class="attention" aria-labelledby="h-attention" data-testid="needs-attention">
			<h2 id="h-attention"><Icon name="alert" size={17} />Needs attention</h2>
			<ul>
				{#each attention as a (a.href + a.text)}
					<li>
						<span>{a.text}</span>
						<a class="fix" href={to(a.href)} aria-label="Fix: {a.text}">Fix<Icon name="chevronRight" size={14} /></a>
					</li>
				{/each}
			</ul>
		</section>
	{/if}

	<div class="settings-overview">
		{#each groups as { group, items } (group.id)}
			{@const gs = answer ? groupState(answer, group.id, items.map((i) => i.href)) : null}
			<section class="group-card" aria-labelledby="g-{group.id}" data-testid="settings-group" data-group={group.id}>
				<header>
					<span class="group-icon"><Icon name={group.icon} size={18} /></span>
					<div class="group-copy">
						<h2 id="g-{group.id}">{group.label}</h2>
						<p>{group.about}</p>
					</div>
					{#if gs && stateWords(gs)}<span class="chip {tone(gs.state)}" data-testid="group-status">{stateWords(gs)}</span>{/if}
				</header>
				<ul class="links">
					{#each items as section (section.href)}
						{@const ps = answer ? pageState(answer, section.href) : null}
						<li>
							<a class="row-link" href={to(section.href)}>
								<span class="setting-icon"><Icon name={section.icon} size={17} /></span>
								<span class="setting-copy"><strong>{section.label}</strong><span>{section.about}</span></span>
								{#if ps && (ps.state === 'attention' || ps.state === 'unset')}<span class="row-status {tone(ps.state)}">{stateWords(ps)}</span>{/if}
								<Icon name="chevronRight" size={16} />
							</a>
						</li>
					{/each}
				</ul>
			</section>
		{/each}
	</div>
{/if}

<style>
	.find { position: relative; display: flex; align-items: center; max-width: 560px; margin: -8px 0 24px; }
	.find > :global(.icon) { position: absolute; left: 14px; color: var(--text-muted); pointer-events: none; }
	.find .input { min-height: 44px; padding-left: 40px; border-radius: var(--r-pill); background: var(--surface-2); border-color: transparent; font: var(--t-body); }
	.attention { margin-bottom: 20px; padding: 14px 18px; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1.5px var(--wait); }
	.attention h2 { display: flex; align-items: center; gap: 8px; font: var(--t-headline); color: var(--wait-text); }
	.attention ul { margin-top: 6px; }
	.attention li { display: flex; align-items: center; gap: 12px; min-height: 44px; }
	.attention li + li { border-top: 1px solid var(--line); }
	.attention li > span { flex: 1; min-width: 0; font: var(--t-callout); overflow-wrap: anywhere; }
	.fix { display: inline-flex; align-items: center; gap: 2px; min-height: 44px; padding: 0 6px 0 12px; color: var(--accent-text); font: var(--t-callout); font-weight: 600; }
	.settings-overview { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 360px), 1fr)); gap: 16px; align-items: start; }
	.group-card { border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); padding: 16px 16px 8px; }
	.group-card header { display: flex; gap: 12px; align-items: flex-start; padding: 2px 4px 8px; }
	.group-icon { width: 34px; height: 34px; flex: none; display: grid; place-items: center; border-radius: 9px; background: var(--accent-soft); color: var(--accent-text); }
	.group-copy { flex: 1; min-width: 0; }
	.group-copy h2 { font: var(--t-headline); font-size: 17px; }
	.group-copy p { margin-top: 2px; color: var(--text-muted); font: var(--t-callout); }
	.group-card .chip { flex: none; white-space: nowrap; }
	.links { margin: 0 -8px; }
	.results { margin: 0; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); padding: 6px; }
	li + li { border-top: 1px solid var(--line); }
	.row-link {
		display: flex;
		align-items: center;
		gap: 12px;
		min-height: 56px;
		padding: 8px 12px;
		border-radius: var(--r-btn);
		transition: background var(--dur-fast) var(--ease-out);
	}
	.row-link:hover { background: var(--surface-2); }
	.setting-icon { width: 30px; height: 30px; flex: none; display: grid; place-items: center; border-radius: 8px; background: var(--surface-2); color: var(--text); }
	.setting-copy { display: block; min-width: 0; flex: 1; }
	.setting-copy strong, .setting-copy span { display: block; }
	.setting-copy strong { font: var(--t-headline); font-size: 15px; }
	.setting-copy span { margin-top: 2px; color: var(--text-muted); font: var(--t-subhead); font-weight: 400; display: -webkit-box; overflow: hidden; line-clamp: 2; -webkit-line-clamp: 2; -webkit-box-orient: vertical; }
	.row-status { flex: none; font: var(--t-footnote); font-weight: 600; }
	.row-status.fail { color: var(--fail-text); }
	.row-status.wait { color: var(--wait-text); }
	.row-link > :global(.icon:last-child) { flex: none; color: var(--text-muted); }
	/* Phones: iOS-style grouped rows. */
	@media (max-width: 767px) {
		.settings-overview { gap: 20px; }
		.group-card { padding: 0; background: none; box-shadow: none; }
		.group-card header { padding: 0 4px 8px; }
		.group-copy p { display: none; }
		.links { margin: 0; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); padding: 2px 4px; }
		.links .setting-copy span { display: none; }
		.row-link { min-height: 52px; }
	}
</style>
