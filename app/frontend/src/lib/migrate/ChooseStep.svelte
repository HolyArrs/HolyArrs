<script lang="ts" module>
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	let keepOpen = false;
</script>

<script lang="ts">
	import { getContext as moveContext } from 'svelte';
	import SettingsCard from '#lib/ui/SettingsCard.svelte';
	import Button from '#lib/ui/Button.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import EmptyState from '#lib/ui/EmptyState.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import type { ShowList } from './ItemsSheet.svelte';
	import { appName, cleanPath, folderProblem, folderTo, OTHER, sharedTargets, type PathRule, type PersonChoice, type Plan, type PlanGroup, type PlanItem } from './move.ts';

	const countSteps = moveContext<boolean | undefined>('move-hide-count') !== true;

	let {
		plan,
		planning = false,
		folders,
		picks,
		others,
		extra,
		extraErrors,
		suggested,
		missing = {},
		seen = [],
		skip,
		people,
		onfolder,
		onother,
		onextra,
		ongroup,
		onperson,
		oncommit,
		onshow
	}: {
		plan: Plan | null;
		planning?: boolean;
		folders: { path: string; kind: string; label: string }[];
		picks: Record<string, string>;
		others: Record<string, string>;
		extra: PathRule[];
		extraErrors: string[];
		suggested: Record<string, string>;
		missing?: Record<string, string>;
		seen?: string[];
		skip: string[];
		people: Record<string, PersonChoice>;
		onfolder: (from: string, pick: string) => void;
		onother: (from: string, to: string) => void;
		onextra: (rules: PathRule[]) => void;
		ongroup: (name: string, on: boolean) => void;
		onperson: (key: string, c: PersonChoice | null) => void;
		oncommit: () => void;
		onshow: ShowList;
	} = $props();

	let open = $state(keepOpen);
	let advOpen = $state(false);
	let peopleOpen = $state(false);
	const defaulted = new Set<string>();
	let held = $state<string[]>([]);
	let keysOpen = $state(false);
	let nmOpen = $state(false);
	let nmApp = $state<string | null>(null);
	let keysOf = $state<'sources' | 'downloaders'>('sources');

	const big = ['titles', 'wishlist', 'people'];
	// The word beside a count (the number is its own element), one and many.
	const counted = (): Record<string, [string, string]> => ({ titles: [t('migrate.title2'), t('migrate.titles3')], wishlist: [t('migrate.request2'), t('migrate.requests3')], people: [t('migrate.person2'), t('migrate.people3')] });
	// Read in "Also: folders, naming and alerts".
	const named = (): Record<string, string> => ({ folders: t('migrate.folders2'), pick_rules: t('migrate.pickRules2'), never_add: t('migrate.neverAddList2'), outside_lists: t('migrate.outsideLists3'), naming: t('migrate.naming2'), subtitles: t('migrate.subtitles2'), sources: t('migrate.sources3'), downloaders: t('migrate.downloaders3'), alerts: t('migrate.alerts3'), other: t('migrate.otherSettings2') });
	const clean = (n: string) => n.replace(/\s*\((Prowlarr|Jackett)\)$/i, '');

	const kindOf = $derived(new Map((plan?.groups.find((g) => g.name === 'folders')?.items ?? []).map((it) => [it.folder?.old_path ?? '', it.folder?.kind ?? ''])));
	const oldRows = $derived((plan?.old_paths ?? []).filter((p) => !folders.some((f) => f.path === p)));
	const shown = $derived((plan?.groups ?? []).filter((g) => g.items.length > 0 || skip.includes(g.name)));
	const emptyGroups = $derived((plan?.groups ?? []).filter((g) => g.items.length === 0 && !skip.includes(g.name) && g.name !== 'other'));
	const on = $derived(shown.filter((g) => g.items.length > 0 && !skip.includes(g.name)));
	const peopleItems = $derived(plan?.groups.find((g) => g.name === 'people')?.items ?? []);
	const peopleOn = $derived(peopleItems.length > 0 && !skip.includes('people'));
	const asked = $derived(peopleItems.filter((it) => it.person?.was_owner || it.person?.was_admin));
	const pathRules = $derived(!!plan && oldRows.length === 0 && (extra.length > 0 || plan.not_moved.some((n) => n.group === 'titles')));
	const needFolders = $derived(oldRows.filter((p) => held.includes(p) || !picks[p]));
	const shared = $derived(sharedTargets(picks, others));
	// The last plan's word on this row's new folder, while the row still points there.
	const goneFor = (p: string) => (missing[p] && cleanPath(missing[p]) === cleanPath(folderTo(picks[p], others[p])) ? missing[p] : '');
	const lead = $derived.by(() => {
		const b = big.map((n) => on.find((g) => g.name === n)).filter((g): g is PlanGroup => !!g);
		if (b.length > 0 || on.length === 0) return b;
		return [on.reduce((a, g) => (g.items.length > a.items.length ? g : a))];
	});
	const tiles = $derived(lead.map((g) => { const w = counted()[g.name]; return { n: g.items.length, w: w ? (g.items.length === 1 ? w[0] : w[1]) : g.label }; }));
	const restLine = $derived.by(() => {
		const rest = on.filter((g) => !lead.includes(g)).map((g) => named()[g.name] ?? g.label);
		return rest.length ? t('migrate.also', { list: andList(rest) }) : '';
	});
	const nmApps = $derived.by(() => {
		const m = new Map<string, number>();
		for (const x of plan?.not_moved ?? []) {
			const k = x.from.split(' ')[0] ?? '';
			m.set(k, (m.get(k) ?? 0) + 1);
		}
		return [...m].sort((a, b) => b[1] - a[1]);
	});
	const nmReasons = $derived.by(() => {
		const m = new Map<string, { what: string; why: string; n: number }>();
		for (const x of plan?.not_moved ?? []) {
			if ((x.from.split(' ')[0] ?? '') !== nmApp) continue;
			const k = x.what + '|' + x.why;
			const r = m.get(k);
			if (r) r.n++;
			else m.set(k, { what: x.what, why: x.why, n: 1 });
		}
		return [...m.values()].sort((a, b) => b.n - a.n);
	});
	const keyCount = $derived({
		sources: on.find((g) => g.name === 'sources')?.items.filter((it) => it.needs).length ?? 0,
		downloaders: on.find((g) => g.name === 'downloaders')?.items.filter((it) => it.needs).length ?? 0
	});
	const keyGroups = $derived.by(() => {
		const src = on.find((g) => g.name === 'sources')?.items.filter((it) => it.needs) ?? [];
		const dls = on.find((g) => g.name === 'downloaders')?.items.filter((it) => it.needs) ?? [];
		const usenet = src.filter((it) => /newznab|usenet/i.test(it.detail ?? ''));
		const torrent = src.filter((it) => !usenet.includes(it));
		const need = (it: PlanItem) => (/password/i.test(it.needs ?? '') ? t('migrate.passwordLabel') : t('migrate.key'));
		return [
			{ id: 'usenet', label: t('migrate.usenet'), items: usenet.map((it) => ({ key: it.key, name: clean(it.name), need: '' })) },
			{ id: 'torrent', label: t('migrate.torrent'), items: torrent.map((it) => ({ key: it.key, name: clean(it.name), need: '' })) },
			{ id: 'downloaders', label: t('migrate.downloaders'), items: dls.map((it) => ({ key: it.key, name: clean(it.name), need: need(it) })) }
		].filter((g) => g.items.length > 0 && (keysOf === 'downloaders' ? g.id === 'downloaders' : g.id !== 'downloaders'));
	});

	$effect(() => {
		const owners = asked.filter((it) => it.person?.was_owner);
		if (owners.length !== 1) return;
		const k = owners[0].key;
		if (defaulted.has(k)) return;
		defaulted.add(k);
		if (people[k] === undefined) onperson(k, 'owner');
	});
	$effect(() => {
		if (planning) return;
		const add = oldRows.filter((p) => !picks[p] && !held.includes(p));
		if (add.length > 0) held = [...held, ...add];
	});
	$effect(() => {
		if (oldRows.some((p) => !needFolders.includes(p) && (folderProblem(p, picks[p] ?? '', others[p]) !== '' || goneFor(p) || shared[p]))) open = true;
		if (extraErrors.some((e) => e !== '')) {
			open = true;
			advOpen = true;
		}
	});

	function toggle() {
		open = !open;
		keepOpen = open;
	}
	function andList(xs: string[]): string {
		return xs.length < 2 ? (xs[0] ?? '') : t('migrate.and', { list: xs.slice(0, -1).join(', '), last: xs[xs.length - 1] });
	}
	function countOf(g: PlanGroup): string {
		const w = counted()[g.name];
		return w ? `${g.items.length} ${g.items.length === 1 ? w[0] : w[1]}` : `${g.label}: ${g.items.length}`;
	}
	function choicesFor(p: string): { value: string; label: string }[] {
		const k = kindOf.get(p);
		const out = folders.filter((f) => !k || !f.kind || f.kind === k).map((f) => ({ value: f.path, label: f.path }));
		const s = suggested[p];
		if (s && !out.some((o) => o.value === s)) out.unshift({ value: s, label: s });
		for (const f of seen) if (!out.some((o) => o.value === f)) out.push({ value: f, label: f });
		return out;
	}
	function groupNote(g: PlanGroup): { text: string; warn: boolean } {
		if (skip.includes(g.name)) return { text: t('migrate.leftOut'), warn: false };
		const n = g.items.length;
		const needs = g.items.filter((it) => it.needs);
		if (needs.length > 0 && g.name === 'people') return { text: t('migrate.inviteAfterTheMove'), warn: true };
		if (needs.length > 0) return { text: needs.every((it) => /key/i.test(it.needs ?? '')) ? t('migrate.keysAfterTheMove') : t('migrate.keysOrPasswordsAfter'), warn: true };
		void n;
		return { text: '', warn: false };
	}
	function itemRow(it: PlanItem) {
		const hint = [serverWords(it.why || it.detail), it.needs ? t('migrate.needs', { what: serverWords(it.needs) }) : ''].filter(Boolean).join(' · ');
		return { key: it.key, name: serverWords(it.name), hint };
	}
	function openKeys(k: 'sources' | 'downloaders') {
		keysOf = k;
		keysOpen = true;
	}
	function personRow(it: PlanItem) {
		const me = people[it.key] === 'owner';
		return { key: it.key, name: it.name, pill: me ? { word: t('migrate.you'), tone: 'ok' as const } : { word: t('migrate.invite'), tone: 'quiet' as const } };
	}
	function personHint(it: PlanItem): string {
		const app = appName(it.person?.app ?? it.from.split(' ')[0] ?? '');
		if (it.person?.was_owner) return t('migrate.ownerInTheirRequests', { app });
		return t('migrate.adminInTurnOff', { app });
	}
	function keyDone(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			e.preventDefault();
			oncommit();
		}
	}
	function clipMiddle(p: string, max = 42): string {
		if (p.length <= max) return p;
		const half = Math.floor((max - 1) / 2);
		return p.slice(0, half) + '…' + p.slice(p.length - half);
	}
</script>

{#snippet folderRow(p: string, n: number, id: string, testid: string, note: boolean)}
	{@const pick = picks[p] ?? ''}
	{@const err = folderProblem(p, pick, others[p])}
	{@const gone = err ? '' : goneFor(p)}
	{@const twins = shared[p] ?? []}
	<li class="mrow wrap" data-testid={testid}>
		<div class="txt">
			<label for={id} class="mono path one" title={p}><span aria-hidden="true">{clipMiddle(p)}</span><span class="sr-only">{p}</span></label>
			{#if note && pick && pick === suggested[p]}<span class="sub">{t('migrate.suggested')}</span>{/if}
		</div>
		<span class="arrow" aria-hidden="true">→</span>
		<select {id} class="input pick" value={pick} aria-invalid={err ? 'true' : undefined} aria-describedby={err ? `${id}-err` : undefined} onchange={(e) => onfolder(p, e.currentTarget.value)}>
			<option value="">{t('migrate.donTBringThem')}</option>
			{#each choicesFor(p) as o (o.value)}<option value={o.value}>{o.label}</option>{/each}
			<option value={OTHER}>{t('migrate.otherPath')}</option>
		</select>
		{#if pick === OTHER || err || gone || twins.length}
			<div class="below">
				{#if pick === OTHER}
					<label class="sr-only" for="{id}-other">{t('migrate.pathHereFor', { path: p })}</label>
					<input id="{id}-other" class="input mono wide" value={others[p] ?? ''} placeholder="/data/movies" aria-invalid={err ? 'true' : undefined} aria-describedby={err ? `${id}-err` : undefined} oninput={(e) => onother(p, e.currentTarget.value)} onblur={oncommit} onkeydown={keyDone} />
				{/if}
				{#if err}<p class="form-error" id="{id}-err">{err}</p>{/if}
				{#if gone}<p class="sub warn note" data-testid="folder-not-here">{t('migrate.notInContainer', { path: gone })}{#if seen.length} {t('migrate.foldersItSees', { list: seen.join(', ') })}{/if}</p>{/if}
				{#if twins.length}<p class="sub warn note" data-testid="folder-shared">{t('migrate.sameNewFolder', { list: twins.join(', ') })}</p>{/if}
			</div>
		{/if}
	</li>
{/snippet}

{#snippet ruleRows()}
	{#each extra as r, i (i)}
		{@const err = extraErrors[i] ?? ''}
		<li class="mrow wrap rule">
			<span class="sr-only">{t('migrate.pathRule', { n: i + 1 })}</span>
			<label class="sr-only" for="move-rule-from-{i}">{t('migrate.oldAppSPath')}</label>
			<input id="move-rule-from-{i}" class="input mono half" value={r.from} placeholder="/movies" aria-invalid={err ? 'true' : undefined} aria-describedby={err ? `move-rule-${i}-err` : undefined} oninput={(e) => onextra(extra.map((x, j) => (j === i ? { ...x, from: e.currentTarget.value } : x)))} onblur={oncommit} onkeydown={keyDone} />
			<span class="arrow" aria-hidden="true">→</span>
			<label class="sr-only" for="move-rule-to-{i}">{t('migrate.pathHere')}</label>
			<input id="move-rule-to-{i}" class="input mono half" value={r.to} placeholder="/data/movies" aria-invalid={err ? 'true' : undefined} aria-describedby={err ? `move-rule-${i}-err` : undefined} oninput={(e) => onextra(extra.map((x, j) => (j === i ? { ...x, to: e.currentTarget.value } : x)))} onblur={oncommit} onkeydown={keyDone} />
			<IconButton icon="x" label={t('migrate.removePathRule', { n: i + 1 })} onclick={() => (onextra(extra.filter((_, j) => j !== i)), oncommit())} />
			{#if err}<div class="below"><p class="form-error" id="move-rule-{i}-err">{err}</p></div>{/if}
		</li>
	{/each}
	<li class="mrow"><Button icon="plus" variant="plain" onclick={() => onextra([...extra, { from: '', to: '' }])}>{t('migrate.addAPathRule')}</Button></li>
{/snippet}

<div class="choose" class:dim={planning && !!plan} aria-busy={planning}>
	<SettingsCard id="move-step" title={countSteps ? t('migrate.step2Of4') : t('migrate.whatToMove')}>
		{#if !plan}
			<p class="hint">{t('migrate.updating')}</p>
		{:else}
			{#if shown.length === 0}
				<EmptyState compact icon="check" title={t('migrate.nothingNewToMove2')} text={t('migrate.everythingInTheseReads')} />
			{:else}
				<div class="tiles" data-testid="move-summary">
					{#each tiles as tl (tl.w)}<div class="tile"><b class="num">{tl.n}</b><span class="sub">{tl.w}</span></div>{/each}
					{#if tiles.length === 0}<div class="tile"><span class="sub">{t('migrate.nothingSwitchedOn')}</span></div>{/if}
				</div>
				{#if restLine}<p class="sub also">{restLine}</p>{/if}
				<p class="sub also" data-testid="move-files-stay">{t('migrate.titlesAndSettingsOnly')}</p>
			{/if}

			{#if needFolders.length}
				<h3 class="head">{t('migrate.folders')}</h3>
				<p class="sub">{t('migrate.pickWhereTheseTitles')}</p>
				<ul class="box list" data-testid="needs-you">
					{#each needFolders as p, n (p)}
						{@render folderRow(p, n, `move-need-folder-${n}`, 'need-folder-row', false)}
					{/each}
				</ul>
			{/if}

			{#if peopleOn && asked.length}
				<h3 class="head">{t('migrate.yourAccount')}</h3>
				<ul class="box list">
					{#if asked.length <= 3 || peopleOpen}
						{#each asked as it, n (it.key)}
						{@const owner = !!it.person?.was_owner}
						<li class="mrow" data-testid="person-row">
							<div class="txt"><span class="name one">{it.name}</span><span class="sub one" title={personHint(it)}>{personHint(it)}</span></div>
							<span class="sw-word" aria-hidden="true">{owner ? t('migrate.thisIsMe') : t('migrate.keepAsAdmin')}</span>
							<label class="switch">
								<input
									id="move-person-{n}"
									type="checkbox"
									checked={people[it.key] === (owner ? 'owner' : 'admin')}
									onchange={(e) => onperson(it.key, e.currentTarget.checked ? (owner ? 'owner' : 'admin') : null)}
									aria-label={owner ? t('migrate.thisIsMe2', { name: it.name }) : t('migrate.keepAsAdmin2', { name: it.name })}
								/>
								<span class="sw" aria-hidden="true"></span>
							</label>
						</li>
						{/each}
					{:else}
						<li class="mrow"><span class="txt">{t('migrate.ownersAndAdmins', { n: asked.length })}</span><button type="button" class="quiet" aria-expanded={peopleOpen} onclick={() => (peopleOpen = true)}>{t('migrate.review')}</button></li>
					{/if}
				</ul>
			{/if}

			{#if keyCount.sources || keyCount.downloaders || peopleOn}
				<h3 class="head">{t('migrate.afterTheMove')}</h3>
				<div class="cards">
					{#if keyCount.sources}
						<div class="box card1">
							<span class="txt"><b>{keyCount.sources === 1 ? t('migrate.reconnectSource', { n: keyCount.sources }) : t('migrate.reconnectSources', { n: keyCount.sources })}</b><span class="sub">{t('migrate.theyArrivePausedAdd')}</span></span>
							<button type="button" class="quiet" onclick={() => openKeys('sources')}>{t('migrate.show2')}</button>
						</div>
					{/if}
					{#if keyCount.downloaders}
						<div class="box card1">
							<span class="txt"><b>{keyCount.downloaders === 1 ? t('migrate.reconnectDownloader', { n: keyCount.downloaders }) : t('migrate.reconnectDownloaders', { n: keyCount.downloaders })}</b><span class="sub">{t('migrate.theyArrivePausedAdd2')}</span></span>
							<button type="button" class="quiet" onclick={() => openKeys('downloaders')}>{t('migrate.show2')}</button>
						</div>
					{/if}
					{#if peopleOn}
						<div class="box card1">
							<span class="txt"><b>{peopleItems.length === 1 ? t('migrate.invite1User') : t('migrate.inviteUsers', { n: peopleItems.length })}</b><span class="sub">{t('migrate.theirAccountsComeOver')}</span></span>
							<button type="button" class="quiet" onclick={() => onshow(t('migrate.people'), peopleItems, personRow)}>{t('migrate.show2')}</button>
						</div>
					{/if}
				</div>
			{/if}

			{#if shown.length > 0 || oldRows.length > 0 || pathRules}
				<button type="button" class="quiet more" aria-expanded={open} aria-controls="move-all" onclick={toggle}>
					<Icon name={open ? 'chevronDown' : 'chevronRight'} size={15} /><span>{t('migrate.changeWhatMoves')}</span>
				</button>
			{/if}
			{#if open}
				<div id="move-all" class="all">
					{#if shown.length > 0}
						<ul class="box list">
							{#each shown as g (g.name)}
								{@const note = groupNote(g)}
								<li class="mrow" data-testid="group-row">
									<span class="txt one" title="{g.label} {g.items.length}{note.text ? ` · ${note.text}` : ''}">
										<label class="name" for="move-group-{g.name}">{g.label}</label>
										<span class="count">{g.items.length}</span>{#if note.text}<span class="gnote" class:warn={note.warn}>{note.text}</span>{/if}
									</span>
									<button type="button" class="quiet" onclick={() => onshow(g.label, g.items, itemRow)} disabled={g.items.length === 0} aria-label={t('migrate.show4', { label: g.label })}>{t('migrate.show2')}</button>
									<label class="switch">
										<input id="move-group-{g.name}" type="checkbox" checked={!skip.includes(g.name)} onchange={(e) => ongroup(g.name, e.currentTarget.checked)} aria-label={t('migrate.move3', { label: g.label })} />
										<span class="sw" aria-hidden="true"></span>
									</label>
								</li>
							{/each}
						</ul>
					{/if}

					{#if oldRows.length > 0}
						<h3 class="head" id="move-folders">{t('migrate.folders')}</h3>
						<ul class="box list">
							{#each oldRows.filter((x) => !needFolders.includes(x)) as p, n (p)}
								{@render folderRow(p, n, `move-folder-${n}`, 'folder-row', true)}
							{/each}
						</ul>
						<details class="adv" bind:open={advOpen}>
							<summary><Icon name="chevronRight" size={15} /><span>{t('migrate.advanced')}</span></summary>
							<p class="sub">{t('migrate.ifAnOldApp')}</p>
							<ul class="box list">{@render ruleRows()}</ul>
						</details>
					{:else if pathRules}
						<h3 class="head" id="move-folders">{t('migrate.pathRules')}</h3>
						<p class="sub">{t('migrate.ifAnOldApp')}</p>
						<ul class="box list">{@render ruleRows()}</ul>
					{/if}

					{#if plan.not_moved.length > 0}
						<ul class="box list tail">
							<li class="mrow" data-testid="not-moved-row">
								<span class="txt one"><span class="name">{t('migrate.notMoved')}</span> <span class="sub">{plan.not_moved.length === 1 ? t('migrate.settingWithNoMatch', { n: plan.not_moved.length }) : t('migrate.settingsWithNoMatch', { n: plan.not_moved.length })}</span></span>
								<button type="button" class="quiet" onclick={() => { nmApp = null; nmOpen = true; }}>{t('migrate.show2')}</button>
							</li>
						</ul>
					{/if}
					{#if emptyGroups.length > 0}
						<p class="sub none">{t('migrate.nothingToMoveFor', { list: emptyGroups.map((g) => g.label).join(', ') })}</p>
					{/if}
				</div>
			{/if}
		{/if}
	</SettingsCard>
</div>

<Sheet bind:open={keysOpen} title={keysOf === 'downloaders' ? t('migrate.downloadersToReconnect') : t('migrate.sourcesToReconnect')} size="md">
	<div class="kgroups" data-testid="keys-sheet">
		{#each keyGroups as g (g.label)}
			<p class="khead">{g.label} · {g.items.length}</p>
			<ul class="chips">
				{#each g.items as it (it.key)}<li class="chip">{it.name}{#if it.need}<span class="cneed">{it.need}</span>{/if}</li>{/each}
			</ul>
		{/each}
	</div>
</Sheet>

<Sheet bind:open={nmOpen} title={nmApp ? t('migrate.notMoved2', { name: appName(nmApp) }) : t('migrate.notMoved')} size="md">
	{#if !nmApp}
		<p class="sub nm-lead">{t('migrate.holyarrsHandlesTheseIts')}</p>
		<ul class="box list" data-testid="not-moved-apps">
			{#each nmApps as [k, n] (k)}
				<li><button type="button" class="nm-row" onclick={() => (nmApp = k)}><b>{appName(k) || t('migrate.other')}</b><span class="count">{n}</span><Icon name="chevronRight" size={15} /></button></li>
			{/each}
		</ul>
	{:else}
		<button type="button" class="quiet nm-back" onclick={() => (nmApp = null)}><Icon name="chevronLeft" size={15} />{t('migrate.allApps')}</button>
		<ul class="box list">
			{#each nmReasons as r (r.what + r.why)}
				<li class="mrow"><span class="txt"><span class="name one">{serverWords(r.what)}</span><span class="sub">{serverWords(r.why)}</span></span>{#if r.n > 1}<span class="count">×{r.n}</span>{/if}</li>
			{/each}
		</ul>
	{/if}
</Sheet>

<style>
	.nm-lead { margin: 0 2px 10px; }
	.nm-row { width: 100%; display: flex; align-items: center; gap: 10px; min-height: 48px; padding: 0 16px; border: 0; background: none; color: var(--text); font: var(--t-callout); text-align: left; cursor: pointer; }
	.nm-row b { flex: 1; font-weight: 600; }
	.nm-row:hover { background: color-mix(in srgb, var(--text) 5%, transparent); }
	.nm-row:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 12px; }
	li + li > .nm-row { border-top: 1px solid var(--line); }
	.nm-back { display: inline-flex; align-items: center; gap: 4px; margin: 0 0 10px; }
	.cards { display: grid; gap: 10px; }
	.card1 { display: flex; align-items: center; gap: 12px; padding: 14px 16px; }
	.gnote { margin-left: 10px; color: var(--text-muted); font: var(--t-footnote); }
	.gnote.warn { color: var(--wait-text); }
	.tiles { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; }
	.tile { padding: 14px 16px; border-radius: 16px; background: var(--surface-2); border: 1px solid var(--line); display: flex; flex-direction: column; gap: 2px; min-width: 0; }
	.num { font: 700 26px/1.1 var(--font-ui); color: var(--text); }
	.also { margin: 8px 2px 0; }
	.kgroups { display: flex; flex-direction: column; gap: 6px; padding-bottom: 8px; }
	.khead { margin: 8px 2px 2px; color: var(--text-muted); font: var(--t-footnote); }
	.chips { list-style: none; margin: 0; padding: 0; display: flex; flex-wrap: wrap; gap: 8px; }
	.chip { padding: 7px 12px; border-radius: 10px; background: var(--surface-2); font: 500 14px/1.2 var(--font-ui); color: var(--text); }
	.cneed { margin-left: 6px; color: var(--text-muted); font-weight: 400; font-size: 13px; }
	.choose { display: contents; }
	.dim :global(.ui-settings .body) { opacity: 0.6; transition: opacity var(--dur-fast) var(--ease-out); }

	.box { margin: 0; border-radius: 16px; background: var(--surface-2); border: 1px solid var(--line); }
	.list { list-style: none; padding: 0 16px; }
	.head { margin: 18px 0 8px; font: var(--t-subhead); font-weight: 600; color: var(--text); }
	.sub { color: var(--text-muted); font: var(--t-footnote); font-weight: 400; }
	.warn { color: var(--wait-text); }
	.none { margin: 0; }

	.mrow { display: flex; align-items: center; gap: 10px; min-height: 48px; padding: 6px 0; min-width: 0; }
	.mrow + .mrow { border-top: 1px solid var(--line); }
	.wrap { flex-wrap: wrap; }
	.txt { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; font: var(--t-callout); color: var(--text); }
	span.txt.one { display: block; }
	.one { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.name { font: 600 14px/1.3 var(--font-ui); color: var(--text); }
	.count { color: var(--text-muted); margin-left: 8px; }
	.path { font-size: 13px; color: var(--text); }
	.arrow { flex: none; color: var(--text-muted); }
	.sw-word { color: var(--text-muted); font: var(--t-footnote); white-space: nowrap; }
	.switch { flex: none; }

	select.pick { flex: none; width: 240px; min-height: 36px; height: 36px; }
	.half { flex: 1 1 0; min-width: 0; min-height: 36px; height: 36px; }
	.wide { width: 100%; max-width: 100%; box-sizing: border-box; min-height: 36px; }
	.below { flex-basis: 100%; padding-bottom: 6px; }
	.below .form-error, .below .note { margin: 6px 0 0; }

	.quiet { flex: none; border: 0; padding: 4px 0; background: none; color: var(--text-muted); font: 500 13px/1.2 var(--font-ui); cursor: pointer; }
	.quiet:hover:not(:disabled) { color: var(--text); }
	.quiet:disabled { opacity: 0.5; cursor: default; }
	.quiet:focus-visible, .adv summary:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 6px; }
	.more { display: inline-flex; align-items: center; gap: 4px; margin-top: 14px; }
	.all { margin-top: 10px; }
	.tail { margin-top: 10px; }
	.all > .none { margin-top: 10px; }

	.adv { margin-top: 10px; }
	.adv summary { display: inline-flex; align-items: center; gap: 4px; list-style: none; cursor: pointer; color: var(--text-muted); font: 500 13px/1.2 var(--font-ui); padding: 4px 0; }
	.adv summary::-webkit-details-marker { display: none; }
	.adv summary :global(svg) { transition: transform var(--dur-fast) var(--ease-out); }
	.adv[open] summary :global(svg) { transform: rotate(90deg); }
	.adv > .sub { margin: 6px 0 8px; }

	@media (max-width: 600px) {
		.wrap .txt { flex-basis: 100%; }
		.wrap .txt + .arrow { display: none; }
		select.pick { flex: 1 1 100%; width: auto; min-width: 0; }
	}
	@media (max-width: 600px), (pointer: coarse) {
		.quiet, .adv summary { min-height: 44px; padding: 0 8px; margin: 0 -8px; }
		.more { margin-left: -8px; }
		select.pick, .half, .wide { min-height: 44px; height: 44px; }
	}
</style>
