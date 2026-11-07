<script lang="ts">
	// Feeds the live job poller and shows it as a day column.
	import { onMount } from 'svelte';
	import LiveDay from './LiveDay.svelte';
	import { LiveJobs } from './live-jobs.svelte.ts';
	import type { StatusRow } from './rows.ts';

	let { upcoming, recent }: { upcoming: StatusRow[]; recent: StatusRow[] } = $props();

	const lj = new LiveJobs();
	// The poller learns job names from the rows so live jobs get readable labels.
	$effect(() => lj.learn([...upcoming, ...recent]));
	onMount(() => {
		lj.start();
		return () => lj.stop();
	});
</script>

<div class="lv" data-testid="status-live" data-running={lj.running.length}>
	<LiveDay {lj} {upcoming} {recent} />
</div>

<style>
	.lv { display: flex; flex-direction: column; min-width: 0; min-height: 100%; }
</style>
