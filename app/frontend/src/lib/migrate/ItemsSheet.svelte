<script lang="ts" module>
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	export interface SheetRow {
		key: string;
		name: string;
		hint?: string;
		pill?: { word: string; tone: 'ok' | 'warn' | 'bad' | 'quiet' };
		look?: boolean;
	}
	export type ShowList = <T>(title: string, items: readonly T[], row: (t: T, i: number) => SheetRow) => void;
</script>

<script lang="ts" generics="T">
	import Sheet from '#lib/ui/Sheet.svelte';
	import Button from '#lib/ui/Button.svelte';
	import SegmentedControl from '#lib/ui/SegmentedControl.svelte';
	import StatusPill, { type StatusKind } from '#lib/ui/StatusPill.svelte';

	let {
		open = $bindable(false),
		title,
		items,
		row,
		empty
	}: {
		open?: boolean;
		title: string;
		items: readonly T[];
		row: (t: T, i: number) => SheetRow;
		empty?: string;
	} = $props();

	const PAGE = 100;
	let q = $state('');
	let shown = $state(PAGE);
	let which = $state<'look' | 'all'>('look');
	const uid = $props.id();
	const kinds: Record<string, StatusKind> = { ok: 'in_library', warn: 'missing', bad: 'failed', quiet: 'requested' };

	const rows = $derived.by(() => {
		const out = open ? items.map((t, i) => row(t, i)) : [];
		const look = out.filter((r) => r.look);
		const mixed = look.length > 0 && look.length < out.length;
		return { all: mixed ? [...look, ...out.filter((r) => !r.look)] : out, look, mixed };
	});
	const list = $derived.by(() => {
		const base = rows.mixed && which === 'look' ? rows.look : rows.all;
		const t = q.trim().toLowerCase();
		return t ? base.filter((r) => r.name.toLowerCase().includes(t) || (r.hint ?? '').toLowerCase().includes(t)) : base;
	});
	const page = $derived(list.slice(0, shown));

	$effect(() => {
		if (open) {
			void items;
			q = '';
			shown = PAGE;
			which = 'look';
		}
	});
</script>

<Sheet bind:open {title} size="lg" placement="auto">
	<div class="tools">
		<label class="sr-only" for="{uid}-q">{t('migrate.searchThisList')}</label>
		<input id="{uid}-q" class="input" type="search" placeholder={t('migrate.search')} bind:value={q} oninput={() => (shown = PAGE)} autocomplete="off" />
		{#if rows.mixed}
			<SegmentedControl
				label={t('migrate.whichLines')}
				options={[
					{ value: 'look', label: t('migrate.needsALook') },
					{ value: 'all', label: t('migrate.all') }
				]}
				bind:value={which}
				onchange={() => (shown = PAGE)}
			/>
		{/if}
	</div>
	<p class="hint count" aria-live="polite" data-testid="items-count">{t('migrate.showingOf', { shown: page.length, total: list.length })}</p>
	{#if list.length === 0}
		<p class="hint">{q.trim() ? t('migrate.nothingMatches') : (empty ?? t('migrate.nothingHere'))}</p>
	{:else}
		<div class="rows" data-testid="items-list">
			{#each page as r (r.key)}
				<div class="srow">
					<div class="txt">
						<span class="label name">{r.name}</span>
						{#if r.hint}<span class="hint">{r.hint}</span>{/if}
					</div>
					{#if r.pill}
						<div class="ctl"><span class="pill"><StatusPill kind={kinds[r.pill.tone]} label={r.pill.word} /></span></div>
					{/if}
				</div>
			{/each}
		</div>
		{#if list.length > shown}
			<div class="more"><Button onclick={() => (shown += PAGE)}>{t('migrate.show100More')}</Button></div>
		{/if}
	{/if}
</Sheet>

<style>
	.tools { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
	.tools .input { flex: 1 1 200px; min-width: 0; }
	.count { margin: 8px 0 0; }
	.rows { margin-top: 4px; }
	.srow { display: flex; align-items: center; gap: 12px; padding: 10px 2px; border-top: 1px solid var(--line); }
	.srow:first-child { border-top: 0; }
	.srow .txt { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
	.srow .hint { color: var(--text-muted); font: var(--t-footnote); }
	.srow .ctl { flex: none; }
	.name { display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; overflow-wrap: anywhere; }
	.rows .hint { overflow-wrap: anywhere; }
	.pill { display: flex; justify-content: flex-end; min-width: 0; }
	.more { display: flex; justify-content: center; padding-top: 12px; }
	@media (max-width: 600px) {
		.rows .srow { grid-template-columns: minmax(0, 1fr); }
		.rows .ctl { justify-self: start; }
		.pill { width: auto; }
	}
</style>
