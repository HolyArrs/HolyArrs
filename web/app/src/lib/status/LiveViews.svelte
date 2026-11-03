<script lang="ts">
	// The Timeline plate's right half: one live feed, shown in whichever of
	// the ten shapes ?live= picks (preview; one stays once Luis has chosen).
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { LiveJobs } from './livejobs.svelte.ts';
	import { liveViewOf } from './liveviews.ts';
	import type { StatusRow } from './status.ts';
	import LiveBoard from './live/LiveBoard.svelte';
	import LiveDial from './live/LiveDial.svelte';
	import LiveCountdown from './live/LiveCountdown.svelte';
	import LiveLog from './live/LiveLog.svelte';
	import LiveClock from './live/LiveClock.svelte';
	import LiveRings from './live/LiveRings.svelte';
	import LivePipeline from './live/LivePipeline.svelte';
	import LiveHours from './live/LiveHours.svelte';
	import LiveCounters from './live/LiveCounters.svelte';
	import LiveStreaks from './live/LiveStreaks.svelte';

	let { upcoming, recent }: { upcoming: StatusRow[]; recent: StatusRow[] } = $props();

	const lj = new LiveJobs();
	$effect(() => lj.learn([...upcoming, ...recent]));
	onMount(() => {
		lj.start();
		return () => lj.stop();
	});
	const view = $derived(liveViewOf(page.url.searchParams.get('live')));
	const views = { board: LiveBoard, dial: LiveDial, countdown: LiveCountdown, log: LiveLog, clock: LiveClock, rings: LiveRings, pipeline: LivePipeline, hours: LiveHours, counters: LiveCounters, streaks: LiveStreaks };
	const View = $derived(views[view]);
</script>

<div class="lv" data-testid="status-live" data-view={view} data-running={lj.running.length}>
	<View {lj} {upcoming} {recent} />
</div>

<style>
	.lv { display: flex; flex-direction: column; min-width: 0; min-height: 100%; }
</style>
