<script lang="ts">
	import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
	import Badge from '#lib/ui/Badge.svelte';
	import IconButton from '#lib/ui/IconButton.svelte';
	import { entryHref, itemDot, type Follows } from './calendar.ts';
	import { groupDates, type CalGroup } from './calendar-groups.ts';
	import { rowWords, episodeWords, channelShown, type CaptionLook } from './calendar-captions.ts';
	import FollowWho from './FollowWho.svelte';
	import CalThumb from './CalThumb.svelte';
	import CalWindow from './CalWindow.svelte';
	import type { CalendarEntry } from '#lib/core/types.ts';
	import { to } from '#lib/core/paths.ts';
	import { date } from '#lib/core/format.ts';

	// The List view in three compact layouts (1 Agenda, 2 Two columns, 3 Table). Every row has one height; several dates of one show on a day are one row ("Episodes 194-196"). A click opens the same details window the Cards view uses.
	let {
		days,
		layout,
		today,
		look,
		follows,
		flipping = null,
		onfollow
	}: {
		days: { day: string; items: CalendarEntry[] }[];
		layout: 1 | 2 | 3;
		today: string;
		look: CaptionLook;
		follows: Follows;
		flipping?: number | null;
		onfollow: (e: CalendarEntry, on: boolean) => void;
	} = $props();

	const dotWord = $derived({ have: t('discover.calListRows.inTheLibrary'), missing: t('discover.calListRows.missing'), upcoming: t('discover.calListRows.comingUp') });
	const rows = $derived(days.map((d) => ({ day: d.day, groups: groupDates(d.items) })));
	let sel = $state<CalGroup | null>(null);
	let open = $state(false);

	function pick(ev: MouseEvent, g: CalGroup) {
		if (ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.button !== 0) return;
		ev.preventDefault();
		sel = g;
		open = true;
	}
	const cells = (g: CalGroup) => {
		const w = rowWords(g, look, today);
		const e = g.lead;
		const ep = e.kind === 'episode';
		const epw = ep ? episodeWords(g.entries) : '';
		const parts = ep ? [w.lines.find((l) => l === epw) ?? '', w.lines.find((l) => l !== e.network && l !== epw) ?? ''] : w.lines.slice(0, 2);
		const [a, b] = parts.filter(Boolean).concat(['', '']);
		const sub = [a, b].filter(Boolean).join(' · ');
		const chip = ep ? (channelShown(e.network, look) ? (e.network ?? '') : '') : e.kind === 'album' ? '' : (e.genres?.[0] ?? '');
		return { w, sub, a, b, chip, dot: itemDot(e, today) };
	};
	const at = (day: string) => new Date(day + 'T00:00:00');
</script>

<div class="lr lay{layout}" data-testid="cal-list" data-layout={layout}>
	{#each rows as d (d.day)}
		{@const when = at(d.day)}
		<section class="day" class:today={d.day === today} data-day={d.day}>
			<header class="dh">
				<p class="sr-only">{date(d.day + 'T12:00:00')}{d.day === today ? `, ${t('discover.calListRows.today2')}` : ''}</p>
				<span class="dow" aria-hidden="true">{when.toLocaleDateString(appLocale(), { weekday: 'short' })}</span>
				<b aria-hidden="true">{when.getDate()}</b>
				<span class="mon" aria-hidden="true">{when.toLocaleDateString(appLocale(), { month: 'short' })}</span>
				{#if d.day === today}<span class="tag"><Badge label={t('discover.calListRows.today')} /></span>{/if}
				<span class="n" aria-hidden="true">{d.groups.length}</span>
			</header>
			<ul class="rws">
				{#each d.groups as g (g.key)}
					{@const c = cells(g)}
					{@const e = g.lead}
					<li class="rw k-{e.kind}" data-testid="cal-item">
						<a class="hit" href={to(entryHref(e))} onclick={(ev) => pick(ev, g)} aria-haspopup="dialog" aria-label="{e.name}, {c.w.when}{c.sub ? ', ' + c.sub : ''}"></a>
						<span class="time" class:quiet={c.w.quiet} data-testid="cal-when">{c.w.when}</span>
						<span class="pic"><CalThumb {e} w={layout === 3 ? 80 : 160} /></span>
						<span class="main">
							<span class="ttl">{c.w.title}</span>
							<span class="sub" data-testid="cal-cap"><span class="s1"><span class="sd {c.dot}" title={dotWord[c.dot]} aria-hidden="true"></span><span class="sr-only">{dotWord[c.dot]}. </span>{#if layout === 2}<span class="tp">{c.w.when}&nbsp;·&nbsp;</span>{/if}{c.a}</span>{#if c.b}<span class="sep">&nbsp;·&nbsp;</span><span class="s2">{c.b}</span>{/if}{#if follows === 'everyone'}<span class="who"><FollowWho {e} size={14} /></span>{/if}</span>
						</span>
						<span class="net">{#if c.chip}<span>{c.chip}</span>{/if}</span>
						<span class="bell" class:on={e.following}>
							<IconButton icon={e.following ? 'bellOn' : 'bell'} size="sm" tipSide="below-end" label={e.following ? t('discover.calListRows.unfollow', { name: e.name }) : t('discover.calListRows.follow', { name: e.name })} aria-pressed={!!e.following} disabled={flipping === e.title_id} onclick={() => onfollow(e, !e.following)} data-testid="cal-follow" />
						</span>
					</li>
				{/each}
			</ul>
		</section>
	{/each}
</div>

{#if sel}<CalWindow bind:open g={sel} {today} {look} />{/if}

<style>
	.lr { --rh: 56px; --stick: var(--topbar); container-type: inline-size; display: flex; flex-direction: column; gap: 14px; }
	:global(.cal-body.barred) .lr { --stick: calc(var(--topbar) + 56px); }
	/* When the top bar slides away the slim bar goes with it, so day headers follow to avoid rows showing through the gap. Phones keep their bars. */
	@media (min-width: 641px) {
		:global(html.bar-hidden) .lr { --stick: 0px; }
		:global(html.bar-hidden .cal-body.barred) .lr { --stick: 53px; }
		.dh { transition: top var(--bar-glide); }
	}
	.day { min-width: 0; }
	.rws { list-style: none; margin: 0; padding: 0; }
	/* Day header, shared by all three layouts. */
	.dh { position: sticky; top: var(--stick); z-index: 5; display: flex; align-items: center; gap: 8px; height: 32px; padding: 0 14px; background: var(--bg); color: var(--text-muted); font: var(--t-footnote); }
	.dh .dow { color: var(--accent-text); font-weight: 600; min-width: 3ch; }
	.dh b { color: var(--text); font: var(--t-headline); font-variant-numeric: tabular-nums; }
	.dh .tag { display: inline-flex; }
	.dh .n { margin-left: auto; font-variant-numeric: tabular-nums; }
	.rw { position: relative; display: grid; align-items: center; height: var(--rh); min-width: 0; }
	.hit { position: absolute; inset: 0; z-index: 1; border-radius: inherit; }
	.hit:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.bell { position: relative; z-index: 2; justify-self: center; }
	.bell :global(.ui-ibtn) { color: var(--text-faint); transition: color var(--dur-fast) var(--ease-out); }
	.rw:hover .bell :global(.ui-ibtn), .bell :global(.ui-ibtn:focus-visible) { color: var(--text-muted); }
	.bell.on :global(.ui-ibtn) { color: var(--accent-text); }
	.time { font: var(--t-subhead); font-weight: 600; font-variant-numeric: tabular-nums; color: var(--text); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.time.quiet { color: var(--text-muted); font-weight: 500; }
	.main { display: flex; flex-direction: column; justify-content: center; gap: 1px; min-width: 0; }
	.ttl { font: var(--t-headline); font-size: 14px; line-height: 20px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; color: var(--text); }
	.sub { display: block; color: var(--text-muted); font: var(--t-footnote); line-height: 18px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.sd { display: inline-block; width: 6px; height: 6px; margin-right: 6px; border-radius: 50%; vertical-align: 1px; background: var(--text-faint); }
	.sd.have { background: var(--ok); }
	.sd.missing { background: var(--fail); }
	.sd.upcoming { background: var(--info); }
	.who { margin-left: 8px; }
	.tp { display: none; font-weight: 600; color: var(--text); }
	.net { min-width: 0; text-align: right; }
	.net > span { display: inline-block; max-width: 100%; padding: 2px 9px; border-radius: 999px; background: var(--surface-2); color: var(--text-muted); font: var(--t-caption); line-height: 16px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; vertical-align: middle; }
	.pic { display: block; overflow: hidden; }
	.rw:hover .ttl { text-decoration: underline; text-underline-offset: 3px; }

	.lay1 { max-width: 800px; }
	.lay1 .day { border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); }
	.lay1 .dh { background: var(--surface-1); border-radius: var(--r-panel) var(--r-panel) 0 0; border-bottom: 1px solid var(--line); }
	.lay1 .rw { grid-template-columns: 76px 80px minmax(0, 1fr) 128px 40px; gap: 14px; padding: 0 6px 0 14px; }
	.lay1 .rw + .rw { border-top: 1px solid var(--line); }
	.lay1 .pic { width: 80px; height: 45px; border-radius: 6px; }
	.lay1 .rw:last-child { border-radius: 0 0 var(--r-panel) var(--r-panel); }
	.lay1 .rw:hover { background: var(--surface-2); }

	.lr.lay2 { --rh: 64px; gap: 18px; }
	.lay2 .day { display: grid; grid-template-columns: 64px minmax(0, 1fr); column-gap: 14px; align-items: start; }
	.lay2 .dh { grid-column: 1; flex-direction: column; justify-content: flex-start; gap: 0; height: auto; padding: 8px 0; align-self: start; background: transparent; }
	.lay2 .dh .dow { min-width: 0; }
	.lay2 .dh b { font: var(--t-title2); line-height: 1.1; }
	.lay2 .dh .mon { font: var(--t-caption); }
	.lay2 .dh .tag { margin-top: 6px; }
	.lay2 .dh .n { display: none; }
	.lay2 .rws { grid-column: 2; display: grid; grid-template-columns: minmax(0, 1fr); gap: 8px; }
	.lay2 .rw { grid-template-columns: 96px minmax(0, 1fr) 64px 40px; gap: 12px; padding: 0 4px 0 0; border-radius: 12px; background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); }
	.lay2 .rw:hover { background: var(--surface-2); }
	.lay2 .pic { grid-column: 1; grid-row: 1; width: 96px; height: 100%; border-radius: 12px 0 0 12px; }
	.lay2 .main { grid-column: 2; grid-row: 1; }
	.lay2 .time { grid-column: 3; grid-row: 1; text-align: right; font-size: 13px; }
	.lay2 .net { display: none; }
	.lay2 .bell { grid-column: 4; grid-row: 1; }
	@container (min-width: 900px) { .lay2 .rws { grid-template-columns: repeat(2, minmax(0, 1fr)); } }

	.lr.lay3 { --rh: 46px; gap: 6px; }
	.lay3 .dh { border-bottom: 1px solid var(--line); padding: 0 8px; }
	.lay3 .dh .dow { color: var(--text-muted); }
	.lay3 .rw { grid-template-columns: 76px 36px minmax(0, 1.1fr) minmax(0, 1fr) minmax(0, 1.4fr) 128px 40px; gap: 14px; padding: 0 0 0 8px; border-radius: 8px; }
	.lay3 .rw:hover { background: var(--surface-1); }
	.lay3 .pic { width: 36px; height: 20px; border-radius: 4px; }
	.lay3 .main { display: contents; }
	.lay3 .ttl { grid-column: 3; }
	.lay3 .time { grid-column: 1; }
	.lay3 .pic { grid-column: 2; }
	.lay3 .net { grid-column: 6; }
	.lay3 .bell { grid-column: 7; }
	.lay3 .sub { display: contents; }
	.lay3 .s1, .lay3 .s2 { display: block; min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; color: var(--text-muted); font: var(--t-footnote); }
	.lay3 .s1 { grid-column: 4; }
	.lay3 .s2 { grid-column: 5; }
	.lay3 .sep { display: none; }

	@container (max-width: 640px) {
		.lay1 .rw { grid-template-columns: 56px 64px minmax(0, 1fr) 36px; gap: 10px; padding: 0 2px 0 10px; }
		.lay1 .net, .lay3 .net { display: none; }
		.lay1 .pic { width: 64px; height: 36px; }
		.lay1 .dh, .lay3 .dh { padding: 0 10px; }
		.lay2 .day { grid-template-columns: 44px minmax(0, 1fr); column-gap: 8px; }
		.lay2 .rw { grid-template-columns: 76px minmax(0, 1fr) 36px; gap: 10px; }
		.lay2 .pic { width: 76px; }
		.lay2 .time { display: none; }
		.lay2 .tp { display: inline; }
		.lay2 .bell { grid-column: 3; }
		.lr.lay3 { --rh: 52px; }
		.lay3 .rw { grid-template-columns: 56px minmax(0, 1fr) 36px; gap: 10px; padding-left: 4px; }
		.lay3 .pic { display: none; }
		.lay3 .main { display: flex; grid-column: 2; }
		.lay3 .ttl { grid-column: auto; }
		.lay3 .time, .lay3 .net { grid-column: auto; }
		.lay3 .bell { grid-column: 3; }
		.lay3 .sub { display: block; }
		.lay3 .s1, .lay3 .s2 { display: inline; }
		.lay3 .sep { display: inline; }
		.lay3 .bell { grid-column: 3; }
	}
	@media (prefers-reduced-motion: reduce) { .bell :global(.ui-ibtn) { transition: none; } }
</style>
