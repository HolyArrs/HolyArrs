<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import type { Snippet } from 'svelte';
	import { to } from '#lib/core/paths.ts';
	import { bytes, percent, agoLong, duration } from '#lib/core/format.ts';
	import { n, uncheckedLine, type HomeAnswer, type HomeCard } from './home.ts';
	import { sized } from '#lib/core/tmdb-image.ts';
	import { stageOf } from '#lib/downloads/index.ts';
	import { metadataOutage } from '#lib/metadata/index.ts';
	import SettingsTile from '#lib/ui/SettingsTile.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { artGone } from './art-check.ts';
	import Button from '#lib/ui/Button.svelte';
	import type { IconName } from '#lib/ui/Icon.svelte';
	import type { Tint } from '#lib/core/settings.ts';
	import { createHomeOverview } from './home-overview.svelte.ts';
	import { createHomeDisk } from './home-disk.svelte.ts';
	import { createHomeAttention } from './home-attention.svelte.ts';
	import { createHomeDownloads } from './home-downloads.svelte.ts';
	import type { DownloadRow } from '#lib/core/types.ts';
	import type { Health } from '#lib/health/index.ts';

	type Props = {
		overview: ReturnType<typeof createHomeOverview>;
		disk: ReturnType<typeof createHomeDisk>;
		admin: boolean;
		settled: boolean;
		attentionView: ReturnType<typeof createHomeAttention>;
		uncheckedParts: boolean;
		retrying: boolean;
		retry: () => unknown;
		approver: boolean;
		kid: boolean;
		uncheckedAsks: boolean;
		downloading: ReturnType<typeof createHomeDownloads>;
		mineOpen: number;
		mineDeclined: number;
		fetchingAll: DownloadRow[];
		home: HomeAnswer | null;
		uncheckedDl: boolean;
		now: Date;
		uncheckedHealth: boolean;
		health: Health | null;
		picTile: Snippet<[{ c: HomeCard; label: string }, string, boolean]>;
	};
	let { overview, disk, admin, settled, attentionView, uncheckedParts, retrying, retry, approver, kid, uncheckedAsks, downloading, mineOpen, mineDeclined, fetchingAll, home, uncheckedDl, now, uncheckedHealth, health, picTile }: Props = $props();
</script>

{#if overview.showOverview}
	<div class="boards" data-testid="home-overview">
		<!-- A tile's head: the same icon square as the Settings pages, its name, and an arrow when it opens a page. -->
		{#snippet head(icon: IconName, tint: Tint, label: string, go = true)}
			<span class="head"><SettingsTile {icon} {tint} /><span class="lab">{label}</span>{#if go}<span class="go" aria-hidden="true"><Icon name="chevronRight" size={16} /></span>{/if}</span>
		{/snippet}
		<!-- A tile whose numbers have not arrived yet. -->
		{#snippet wait(lines = 2)}
			<span class="sk" aria-hidden="true">{#each Array(lines) as _, i (i)}<i></i>{/each}</span><span class="sr-only">{t('home.loading')}</span>
		{/snippet}
		<!-- Disk: what the Library takes, how much room is left, and one thin line along the foot for how full it is (only when the free room is known). -->
		{#snippet diskTile(cls: string)}
			<a class="tile stat {cls}" class:lined={!!disk.free} href={to(admin ? '/settings/library-folders' : '/health')}>
				{@render head('archive', disk.diskTint, t('home.onDisk2'))}
				{#if !settled}
					{@render wait()}
				{:else}
					<span class="num">{bytes(attentionView.diskTotal).split(' ')[0]}<small>{bytes(attentionView.diskTotal).split(' ')[1]}</small></span>
					<span class="sm" class:fail={disk.diskState === 'error'} class:warn={disk.diskState === 'warning'}>{disk.diskLine}</span>
					{#if disk.free}
						<span class="life" role="img" aria-label={t('home.yourLibraryTakesFree', { size: bytes(attentionView.diskTotal), free: bytes(disk.free.free) })}>
							<i class="used {disk.diskState}" style="width: {disk.diskUsed}%"></i>
						</span>
					{/if}
				{/if}
			</a>
		{/snippet}
		<!-- Sources or Downloaders: how many work, which have a problem, and one pip each along the foot. -->
		{#snippet partTile(label: string, icon: IconName, tint: Tint, p: { total: number; on: number; working: number; off: number; failing: number }, trouble: string, path: string, empty: string)}
			<a class="tile stat" class:lined={settled && !uncheckedParts && p.total > 0} href={to(admin ? path : '/health')}>
				{@render head(icon, tint, label)}
				{#if !settled}
					{@render wait()}
				{:else if uncheckedParts}
					<span class="word">{t('home.couldNotRead')}</span>
					<span class="sm">{t('home.homeCouldNotRead')}</span>
				{:else if !p.total}
					<span class="word">{t('home.noneAdded')}</span>
					<span class="sm">{empty}</span>
				{:else}
					<span class="num" class:warn={p.on > 0 && !p.working}>{n(p.working)}<small>{t('home.ofWorking', { n: n(p.total) })}</small></span>
					<span class="sm" class:warn={!!trouble}>{attentionView.partLine(p, trouble)}</span>
					<span class="life pips" aria-hidden="true">
						{#if p.total <= 12}
							{#each Array(p.total) as _, i (i)}<i class:bad={i >= p.working && i < p.on} class:off={i >= p.on}></i>{/each}
						{:else}
							{#if p.working}<i style="flex-grow: {p.working}"></i>{/if}{#if p.failing}<i class="bad" style="flex-grow: {p.failing}"></i>{/if}{#if p.off}<i class="off" style="flex-grow: {p.off}"></i>{/if}
						{/if}
					</span>
				{/if}
			</a>
		{/snippet}
		<!-- One count tile. -->
		{#snippet stat(key: string, cls: string)}
			{#if key === 'att' && settled && attentionView.unchecked && !attentionView.fixes.length}
				<button type="button" class="tile stat {cls}" class:busy={retrying} disabled={retrying} onclick={retry} aria-busy={retrying || undefined} data-testid="home-attention">
					{@render head('refresh', 'amber', t('home.needsAttention'), false)}
					<span class="word">{t('home.couldNotCheck')}</span>
					<span class="sm">{t('home.tapToTryAgain')}</span>
				</button>
			{:else if key === 'att'}
				<a class="tile stat {cls}" href={to(attentionView.fixHref)} data-testid="home-attention">
					{@render head(attentionView.worst === 'ok' ? 'check' : 'alert', attentionView.attTint, t('home.needsAttention'))}
					{#if !settled}
						{@render wait()}
					{:else if attentionView.fixes.length}
						<span class="num" class:fail={attentionView.worst === 'fail'} class:warn={attentionView.worst === 'wait'}>{n(attentionView.fixes.length)}</span>
						{#if attentionView.fixMore}
							<span class="sm two"><span>{attentionView.fixLine}</span><span>{t('home.andMore', { n: n(attentionView.fixMore) })}</span></span>
						{:else}
							<span class="sm">{attentionView.fixLine}</span>
						{/if}
					{:else}
						<span class="word">{t('home.allGood')}</span>
						<span class="sm">{t('home.downloadsImportsAndChecks')}</span>
					{/if}
				</a>
			{:else if key === 'ask'}
				{@const approving = approver && !kid && attentionView.waiting.length > 0}
				<a class="tile stat {cls}" href={to('/requests')}>
					{@render head('requests', 'orange', approving ? t('home.toApprove') : attentionView.mine.length ? t('home.yourAsks') : t('home.requests'))}
					{#if !settled}
						{@render wait()}
					{:else if uncheckedAsks}
						<span class="word">{t('home.couldNotRead')}</span>
						<span class="sm">{t('home.homeCouldNotRead2')}</span>
					{:else if approving && downloading.waitingPics.length}
						<span class="ask-stack" aria-hidden="true">{#each downloading.waitingPics as w (w.a.id + w.pic)}<img src={sized(w.pic, 'poster', 80)} alt="" decoding="async" use:artGone={() => overview.artFailed(w.a, w.pic)} />{/each}{#if attentionView.waiting.length > downloading.waitingPics.length}<b>+{n(attentionView.waiting.length - downloading.waitingPics.length)}</b>{/if}</span>
						<span class="sm">{t(attentionView.waiting.length === 1 ? 'home.askToLookAt' : 'home.asksToLookAt', { n: n(attentionView.waiting.length) })}</span>
					{:else if approving}
						<span class="num">{n(attentionView.waiting.length)}</span>
						<span class="sm">{t(attentionView.waiting.length === 1 ? 'home.askToLookAt2' : 'home.asksToLookAt2')}</span>
					{:else if mineOpen}
						<span class="num">{n(mineOpen)}</span>
						<span class="sm">{t('home.stillOpen2')}{mineDeclined ? ` · ${t('home.turnedDown', { n: n(mineDeclined) })}` : ''}</span>
					{:else if mineDeclined}
						<span class="num">{n(mineDeclined)}</span>
						<span class="sm">{t('home.turnedDown2')}</span>
					{:else}
						<span class="word">{t('home.nothingWaiting')}</span>
						<span class="sm">{approver && !kid ? t('home.noAsksToApprove') : t('home.askForSomethingOn')}</span>
					{/if}
				</a>
			{:else if key === 'lib'}
				<a class="tile stat {cls}" href={to(overview.libHref)} data-testid="home-library">
					{@render head('library', 'indigo', t('home.library'))}
					<span class="num">{n(overview.libTotal)}</span>
					<span class="sm">{attentionView.libCards.length === 1 ? attentionView.libCards[0].detail || t('home.inYourLibrary', { unit: attentionView.libCards[0].unit }) : attentionView.libCards.map((c) => `${n(c.big)}\u00a0${c.unit}`).join(' · ')}</span>
				</a>
			{:else if key === 'disk'}
				{@render diskTile(cls)}
			{:else if key === 'dl'}
				<a class="tile stat {cls}" href={to('/downloads')}>
					{@render head('download', 'blue', t('home.downloads'))}
					<span class="num">{n(fetchingAll.length)}</span>
					<span class="sm">{downloading.stageWords(downloading.countStages(fetchingAll))}</span>
				</a>
			{:else if key === 'soon'}
				<a class="tile stat {cls}" href={to('/calendar')}>
					{@render head('calendar', 'purple', t('home.comingSoon2'))}
					<span class="num">{n(home?.soon?.length ?? 0)}{(home?.soon?.length ?? 0) >= 20 ? '+' : ''}</span>
					<span class="sm">{t('home.inTheNext30')}</span>
				</a>
			{:else if key === 'new'}
				<a class="tile stat {cls}" href={to(overview.kindHref(overview.recentArrived[0] ?? home?.new_seasons?.[0]))}>
					{@render head('sparkle', 'pink', t('home.newForYou'))}
					<span class="num">{n(overview.recentNew)}</span>
					<span class="sm">{overview.recentArrived.length && home?.new_seasons?.length ? t('home.arrivalsAndNewSeasons') : overview.recentArrived.length ? t('home.arrivedInTheLast') : home?.new_seasons?.length === 1 ? t('home.newSeasonOfA') : t('home.newSeasonsOfShows')}</span>
				</a>
			{:else if key === 'explore'}
				<a class="tile stat {cls}" href={to('/explore')}>
					{@render head('compass', 'green', t('home.explore'))}
					<span class="word">{t('home.findSomethingNew')}</span>
					<span class="sm">{t('home.pickWhatToAsk')}</span>
				</a>
			{:else if key === 'popular'}
				<a class="tile stat {cls}" href={to('/popular')}>
					{@render head('star', 'pink', t('home.newPopular'))}
					<span class="word">{t('home.whatSTrending')}</span>
					<span class="sm">{t('home.top10sAndComing')}</span>
				</a>
			{:else if key === 'calendar'}
				<a class="tile stat {cls}" href={to('/calendar')}>
					{@render head('calendarDay', 'purple', t('home.calendar'))}
					<span class="word">{t('home.whatSNext')}</span>
					<span class="sm">{t('home.airDatesAndReleases')}</span>
				</a>
			{:else}
				<a class="tile stat {cls}" href={to('/search')}>
					{@render head('search', 'graphite', t('home.search'))}
					<span class="word">{t('home.lookAnythingUp')}</span>
					<span class="sm">{t('home.byName')}</span>
				</a>
			{/if}
		{/snippet}

		<div class="bento" class:gone={overview.shown !== 'home'} inert={overview.shown !== 'home'} data-testid="home-board">
			{#each overview.places as pl (pl.key)}
				{#if pl.pic}{@render picTile(pl.pic, pl.shape, pl.shape === 'tall')}{:else if pl.stat}{@render stat(pl.stat, pl.shape)}{/if}
			{/each}
		</div>
		{#if overview.showSystem}
			<div class="bento" class:gone={overview.shown !== 'system'} inert={overview.shown !== 'system'} data-testid="home-system">
				<a class="tile stat big dl" href={to('/downloads')}>
					{@render head('download', 'blue', t('home.downloading4'))}
					{#if !settled}
						{@render wait(3)}
					{:else if uncheckedDl}
						<span class="word">{t('home.couldNotRead')}</span>
						<span class="sm">{t('home.homeCouldNotRead3')}</span>
					{:else}
						{#if downloading.speed}
							<span class="num">{bytes(downloading.speed).split(' ')[0]}<small>{bytes(downloading.speed).split(' ')[1]}/s</small></span>
						{:else}
							<span class="num">{n(downloading.dlCount.down)}<small>{t('home.downloading')}</small></span>
						{/if}
						<!-- The three steps a download goes through, in order. -->
						<ol class="flow" aria-label={t('home.downloadsByStep')}>
							<li><b>{n(downloading.dlCount.waiting)}</b><span>{t('home.waiting3')}</span></li>
							<li><b>{n(downloading.dlCount.down)}</b><span>{t('home.downloading4')}</span></li>
							<li><b>{n(downloading.dlCount.imp)}</b><span>{t('home.importing2')}</span></li>
						</ol>
						{#if fetchingAll.length}
							<ul class="dl-list">
								{#each downloading.dlList as d (d.live_key ?? d.id)}
									{@const moving = ['downloading', 'stalled'].includes(stageOf(d))}
									<li><b dir="auto">{d.title || d.name}</b><span>{downloading.dlState(d)}</span>{#if moving}<i class="pb"><i style="width: {percent(d.progress)}%"></i></i>{/if}</li>
								{/each}
							</ul>
						{:else if downloading.recentDone.length}
							<ul class="dl-list done">
								{#each downloading.recentDone as d (d.id)}
									<li><b dir="auto">{d.title || d.name}</b><span>{t('home.finished', { when: agoLong(d.finished_at, now.getTime()) })}</span></li>
								{/each}
							</ul>
						{:else}
							<span class="quiet">{downloading.notSent ? t('home.notSentByHolyArrs', { n: n(downloading.notSent) }) : t('home.nothingIsDownloadingRight')}{downloading.pausedCount ? ' ' + t('home.paused2', { n: n(downloading.pausedCount) }) : ''}</span>
						{/if}
					{/if}
				</a>
				<section class="tile stat big att" aria-labelledby="h-fix" data-testid="home-problems">
					<header class="head">
						<SettingsTile icon={attentionView.worst === 'ok' ? 'check' : 'alert'} tint={attentionView.attTint} />
						<h2 class="lab" id="h-fix">{t('home.needsAttention')}</h2>
						{#if attentionView.fixes.length}<b class="count {attentionView.worst}">{n(attentionView.fixes.length)}</b>{/if}
					</header>
					{#if !settled}
						{@render wait(3)}
					{:else if attentionView.fixes.length}
						<ul class="fixes">
							{#each attentionView.fixShown as g (g.key)}
								<li>
									<a class="fix" href={to(g.href)}>
										<i class="tone {g.tone}" aria-hidden="true"></i>
										<span class="ft"><span class="sr-only">{g.word}: </span><b dir="auto">{g.title}</b><span dir="auto">{g.reason}</span></span>
										<span class="fa"><span class="act">{g.action}</span><Icon name="chevronRight" size={14} /></span>
									</a>
								</li>
							{/each}
						</ul>
						{#if attentionView.fixHidden}
							<a class="more" href={to(attentionView.fixHidden.href)}>{attentionView.fixHidden.text}<Icon name="chevronRight" size={13} /></a>
						{:else if attentionView.unchecked}
							<span class="rest-ok">{t('home.someChecksCouldNot')}</span>
						{:else if attentionView.fixShown.length <= 2 && attentionView.passedCount}
							<span class="rest-ok"><i class="tone ok" aria-hidden="true"></i>{t(attentionView.passedCount === 1 ? 'home.otherCheckPassed' : 'home.otherChecksPassed', { n: n(attentionView.passedCount) })}</span>
						{/if}
					{:else if attentionView.unchecked}
						<span class="word">{t('home.couldNotCheck')}</span>
						<span class="sm">{uncheckedLine(uncheckedDl, uncheckedHealth || uncheckedParts)}</span>
						<span class="again"><Button size="sm" variant="secondary" busy={retrying} onclick={retry}>{t('home.tryAgain')}</Button></span>
					{:else}
						<!-- All good: what was checked, each with its tick. -->
						{#if attentionView.passed.length}
							<ul class="passed">
								{#each attentionView.passed.slice(0, 8) as w (w)}<li><Icon name="check" size={14} />{w}</li>{/each}
							</ul>
						{/if}
						<span class="word">{t('home.allGood')}</span>
						<span class="sm">{t('home.downloadsImportsAndChecks2')}</span>
					{/if}
				</section>
				{@render diskTile('')}
				{@render partTile(t('home.sources'), 'source', 'purple', attentionView.srcs, attentionView.troubled('source', health?.sources), '/settings/sources', t('home.addASourceTo'))}
				{@render partTile(t('home.downloaders'), 'downloader', 'indigo', attentionView.dls, attentionView.troubled('downloader', health?.downloaders), '/settings/downloaders', t('home.addADownloaderTo'))}
				<a class="tile stat" href={to('/health')}>
					{@render head('clock', 'graphite', t('home.runningFor'))}
					{#if !settled}
						{@render wait()}
					{:else}
						{#if health && !uncheckedParts}<span class="num">{duration(health.uptime_s)}</span>{:else}<span class="word">{t('home.couldNotRead')}</span>{/if}
						{#if health && !uncheckedParts}<span class="sm">{t('home.version', { version: health.version })}</span>{/if}
						<span class="sm meta"><i class="tone" class:ok={!metadataOutage.s.down} class:wait={metadataOutage.s.down} aria-hidden="true"></i>{metadataOutage.s.down ? t('home.metadataServerDown') : t('home.metadataServerOnline')}</span>
					{/if}
				</a>
			</div>
		{/if}
	</div>
{/if}

<style>
	/* The same sizes as Explore: 279px wide cards at a 1536px window, 5px gaps on wide rows, one 32px rhythm between blocks. The sheet colours live here so the tiles and the Welcome card share them. */
	.home {
		container: home / inline-size; --title-item: max(200px, calc((100vw - 96px) / 5.16)); --poster-item: 168px; --home-gap: 32px; display: flex; flex-direction: column; gap: var(--home-gap); padding-bottom: 48px;
		--sheet-a: light-dark(#ffffff, #212126); --sheet-b: light-dark(#f4f5f8, #151518);
		--edge: inset 0 1px 0 light-dark(rgb(255 255 255 / 95%), rgb(255 255 255 / 10%)), inset 0 0 0 1px light-dark(rgb(30 36 58 / 9%), rgb(255 255 255 / 6%));
		--drop: 0 1px 2px light-dark(rgb(20 24 40 / 5%), rgb(0 0 0 / 40%)), 0 18px 32px -20px light-dark(rgb(20 24 40 / 30%), rgb(0 0 0 / 85%));
		--well: light-dark(rgb(20 24 40 / 4%), rgb(255 255 255 / 5%));
		--track: light-dark(rgb(20 24 40 / 9%), rgb(255 255 255 / 12%));
	}

	/* Bento: one grid of tiles. Home has pictures in the roomy places and counts in the small ones; the shape follows how many tiles there are, and every shape fills whole rows. System is the same frame filled with how HolyArrs is running. */
	.boards { position: relative; }
	.boards > .bento { transition: opacity var(--dur) var(--ease-out), translate var(--dur) var(--ease-out); }
	.bento.gone { position: absolute; inset: 0 0 auto; opacity: 0; translate: 0 6px; visibility: hidden; pointer-events: none; transition: opacity var(--dur) var(--ease-out), translate var(--dur) var(--ease-out), visibility 0s linear var(--dur); }
	.bento { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); grid-auto-rows: 150px; grid-auto-flow: row dense; gap: 16px; }

	/* The tile is the Requests card's sheet on its own: lit from above, a hairline, and one long soft shadow. A whisper of grain keeps the flat colour from looking printed. */
	.tile {
		position: relative; isolation: isolate; min-width: 0; min-height: 0; display: flex; flex-direction: column; padding: 16px 18px; border: 0; border-radius: 20px; overflow: hidden;
		color: var(--text); text-align: left; font: inherit; background: linear-gradient(180deg, var(--sheet-a), var(--sheet-b)); box-shadow: var(--edge), var(--drop);
		transition: transform var(--dur) var(--ease-out), box-shadow var(--dur) var(--ease-out), opacity var(--dur-fast) var(--ease-out);
	}
	.stat::before { content: ''; position: absolute; inset: 0; z-index: -1; border-radius: inherit; pointer-events: none; opacity: .05; background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='180' height='180'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='.85' numOctaves='2' stitchTiles='stitch'/%3E%3CfeColorMatrix values='0 0 0 0 .5 0 0 0 0 .5 0 0 0 0 .5 0 0 0 1 0'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E"); }
	a.tile, button.tile { cursor: pointer; }
	a.tile:hover, button.tile:hover { transform: translateY(var(--lift)); box-shadow: var(--edge), 0 2px 4px light-dark(rgb(20 24 40 / 6%), rgb(0 0 0 / 40%)), 0 26px 40px -22px light-dark(rgb(20 24 40 / 38%), rgb(0 0 0 / 90%)); }
	a.tile:focus-visible, button.tile:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.tile.busy { opacity: .6; }
	.tile.big { grid-column: span 2; grid-row: span 2; }
	.tile.tall { grid-row: span 2; }

	.head { display: flex; align-items: center; gap: 10px; min-width: 0; }
	.lab { flex: 1; min-width: 0; margin: 0; font: 600 14px / 1.3 var(--font-ui); letter-spacing: -0.005em; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.go { flex: none; display: inline-grid; color: var(--text-faint); transition: transform var(--dur) var(--ease-out), color var(--dur-fast) var(--ease-out); }
	a.tile:hover .go { color: var(--text); transform: translateX(2px); }
	.num, .word { margin-top: auto; }
	.num { font: 800 40px / 1 var(--font-display); letter-spacing: -0.04em; font-variant-numeric: tabular-nums; white-space: nowrap; }
	.num small { margin-left: 5px; font: 500 13px / 1 var(--font-ui); letter-spacing: 0; color: var(--text-muted); }
	.word { font: 700 24px / 1.1 var(--font-display); letter-spacing: -0.025em; }
	.sm { margin-top: 5px; font: var(--t-footnote); color: var(--text-muted); overflow: hidden; overflow-wrap: anywhere; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; }
	.stat > .num + .sm:not(:has(+ .sm)), .stat > .word + .sm:not(:has(+ .sm)) { min-height: 2.6em; }
	/* Two lines that must both show: the row (cut with an ellipsis if long) and "and N more". */
	.sm.two { display: block; }
	.sm.two > span { display: block; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.fail, .sm.fail { color: var(--fail); }
	.warn, .sm.warn { color: var(--wait); }
	.sk { margin-top: auto; display: flex; flex-direction: column; gap: 8px; }
	.sk i { display: block; height: 12px; border-radius: 6px; background: var(--sk-base); }
	.sk i + i { width: 60%; }
	.sk i + i + i { width: 40%; }

	.life { position: absolute; left: 18px; right: 18px; bottom: 6px; display: flex; gap: 2px; height: 4px; border-radius: 4px; overflow: hidden; background: var(--track); }
	.life i { display: block; min-width: 2px; flex-basis: 0; }
	.life .used { flex: none; border-radius: inherit; background: #1ea9be; }
	.life .used.warning { background: var(--wait); }
	.life .used.error { background: var(--fail); }
	.life.pips { gap: 3px; background: none; }
	.life.pips i { flex: 1; border-radius: 4px; background: var(--ok); }
	.life.pips i.bad { background: var(--wait); }
	.life.pips i.off { background: var(--track); }
	.pic .sm { color: rgb(255 255 255 / 86%); font: 600 12px / 1.3 var(--font-ui); -webkit-line-clamp: 1; line-clamp: 1; }

	.ask-stack { margin-top: auto; display: flex; align-items: center; }
	.ask-stack img { width: 38px; height: auto; aspect-ratio: 2 / 3; object-fit: cover; border-radius: 7px; margin-left: -12px; background: var(--surface-3); box-shadow: 0 0 0 2.5px var(--sheet-b), 0 6px 14px -4px rgb(0 0 0 / 35%); }
	.ask-stack img:first-child { margin-left: 0; }
	.ask-stack b { min-width: 30px; height: 30px; margin-left: 10px; padding: 0 8px; display: grid; place-items: center; border-radius: 999px; background: var(--well); box-shadow: inset 0 0 0 1px var(--line); color: var(--text); font: var(--t-footnote); font-weight: 700; font-variant-numeric: tabular-nums; }

	.dl .num, .dl > .word { margin-top: 14px; }
	.flow { list-style: none; margin: 14px 0 0; padding: 0; display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); border-radius: 12px; background: var(--well); box-shadow: inset 0 0 0 1px var(--line); }
	.flow li { position: relative; min-width: 0; padding: 9px 12px; display: flex; flex-direction: column; gap: 2px; }
	.flow li + li::before { content: ''; position: absolute; left: 0; top: 10px; bottom: 10px; width: 1px; background: var(--line); }
	.flow b { font: 700 18px / 1.1 var(--font-display); letter-spacing: -0.02em; font-variant-numeric: tabular-nums; }
	.flow span { font: var(--t-footnote); color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.dl-list { list-style: none; margin: auto 0 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
	.dl-list li { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 3px 10px; font: var(--t-footnote); }
	.dl-list b { font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.dl-list span { color: var(--text-muted); font-variant-numeric: tabular-nums; white-space: nowrap; }
	.pb { grid-column: 1 / -1; display: block; height: 4px; border-radius: 4px; background: var(--track); overflow: hidden; }
	.pb i { display: block; height: 100%; border-radius: inherit; background: var(--info); }
	.quiet { margin-top: auto; font: var(--t-callout); color: var(--text-muted); }

	/* Dark theme's red and amber are light colours, so the digits go dark there to stay readable. */
	.count { flex: none; min-width: 24px; height: 24px; padding: 0 7px; display: grid; place-items: center; border-radius: 999px; background: var(--fail); color: light-dark(#fff, #17171b); font: 700 12px / 1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.count.wait { background: var(--wait); }
	.fixes { list-style: none; margin: 10px -8px 0; padding: 0; display: flex; flex-direction: column; min-height: 0; }
	.fixes li + li { box-shadow: inset 0 1px 0 var(--line); }
	.fix { display: flex; align-items: center; gap: 12px; min-height: 50px; padding: 0 8px; border-radius: 10px; color: inherit; transition: background var(--dur-fast) var(--ease-out); }
	.fix:hover { background: var(--well); }
	.fix:focus-visible, .more:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.tone { flex: none; width: 8px; height: 8px; border-radius: 50%; background: var(--fail); }
	.tone.wait { background: var(--wait); }
	.tone.ok { background: var(--ok); }
	.ft { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
	.ft b { font: 600 14px / 1.3 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.ft > span:last-child { font: var(--t-footnote); color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.fa { flex: none; display: inline-flex; align-items: center; gap: 2px; color: var(--accent-text); font: 600 13px / 1 var(--font-ui); white-space: nowrap; }
	.more { position: relative; margin-top: auto; align-self: flex-start; display: inline-flex; align-items: center; gap: 2px; min-height: 28px; border-radius: 8px; color: var(--accent-text); font: 600 13px / 1 var(--font-ui); }
	.more::after { content: ''; position: absolute; inset: -8px 0; }
	.more:hover { text-decoration: underline; text-underline-offset: 3px; }
	.rest-ok { margin-top: auto; display: flex; align-items: center; gap: 8px; font: var(--t-footnote); color: var(--text-muted); }
	.passed { list-style: none; margin: 14px 0 0; padding: 0; display: flex; flex-wrap: wrap; gap: 6px; overflow: hidden; }
	.passed li { display: inline-flex; align-items: center; gap: 5px; height: 28px; padding: 0 10px 0 8px; border-radius: 999px; background: var(--well); box-shadow: inset 0 0 0 1px var(--line); font: var(--t-footnote); color: var(--text-muted); white-space: nowrap; }
	.passed li :global(.icon) { color: var(--ok); }
	.again { margin-top: 12px; }
	.meta { display: flex; align-items: center; gap: 6px; margin-top: 0; }

	/* Two columns (a narrow window, a tablet, a phone): the big tiles keep their two-by-two, the tall one becomes one row, an odd last tile spans the row, and everything inside a tile takes its compact size (the rows are 132px here). */
	@container home (max-width: 900px) {
		.bento { grid-template-columns: repeat(2, minmax(0, 1fr)); grid-auto-rows: 132px; gap: 10px; }
		.tile.tall { grid-row: span 1; }
		.tile { padding: 12px 14px; border-radius: 18px; }
		.head { gap: 8px; }
		.lab { font-size: 13px; }
		.num { font-size: 30px; }
		.num small { font-size: 12px; margin-left: 4px; }
		.word { font-size: 19px; }
		.life { left: 14px; right: 14px; bottom: 5px; height: 3px; }
		.sm { line-height: 1.3; }
		.ask-stack img { width: 32px; margin-left: -10px; border-radius: 6px; }
		.ask-stack b { min-width: 26px; height: 26px; margin-left: 8px; }
		.dl .num, .dl > .word { margin-top: 10px; }
		.flow { margin-top: 10px; }
		.flow li { padding: 7px 10px; }
		.fixes { margin-top: 6px; }
		.fix { min-height: 44px; }
		.passed { margin-top: 8px; max-height: 130px; }
}
	/* Phones: the big tiles have less room, so two downloads show and the problem rows keep only their arrow. */
	@container home (max-width: 560px) {
		.dl-list li:nth-child(n + 3) { display: none; }
		.act { display: none; }
}
	/* Two columns on a wide screen: a little taller, so pictures are not thin strips. */
	@container home (min-width: 560px) and (max-width: 900px) {
		.bento { grid-auto-rows: 150px; }
}
	/* A small phone (360): the tile names still fit. */
	@container home (max-width: 340px) {
		.tile { padding-inline: 12px; }
		.life { left: 12px; right: 12px; }
		.head { gap: 7px; }
		.lab { font-size: 12.5px; letter-spacing: -0.012em; }
}
	/* Phones: Search and Explore become two round buttons beside the switch; the note goes under them and keeps its line when there is a switch, so Home and System start at the same height. */
	@media (max-width: 767px) {
		.home { --home-gap: 26px; --poster-item: 54vw; --title-item: calc((100cqi - var(--shelf-gap)) / 1.6 / .78); }
		.go { display: none; }
}
	/* An odd last line takes the whole width, so the sheet has no empty half. */
	.up-th img { width: 100%; height: 100%; object-fit: cover; object-position: center 28%; }
	.up-t b { font: 600 14px / 1.3 var(--font-ui); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.up-t span { font: var(--t-footnote); color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	@container home (max-width: 700px) {
}
	.segs i { height: 4px; border-radius: 2px; background: var(--track); }
	.steps a { display: flex; align-items: center; gap: 12px; min-height: 48px; padding: 0 10px; border-radius: var(--r-btn); color: var(--text); font: var(--t-callout); font-weight: 500; }
	.steps a:hover { background: var(--well); }
	.steps a:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.steps a > :global(.icon) { color: var(--text-muted); }
	.steps .done a { color: var(--text-muted); }

	/* High Contrast (forced colors) drops shadows and fills: tiles, the steps box and the ticks get a system border, focus gets a system outline, and the small coloured marks (dots, pips, the foot line, the count) keep their colours. */
	@media (forced-colors: active) {
		.home :global(:focus-visible) { outline: 2px solid Highlight !important; outline-offset: 2px; }
		.tile, .flow, .passed li { border: 1px solid CanvasText; }
		.life, .pb, .tone, .count, .ask-stack b { forced-color-adjust: none; }
		.life:not(.pips), .pb, .life.pips i.off { outline: 1px solid CanvasText; outline-offset: -1px; }
}
</style>
