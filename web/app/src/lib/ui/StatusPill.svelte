<script lang="ts" module>
	export type StatusKind = 'in_library' | 'downloading' | 'missing' | 'requested' | 'approved' | 'turned_down' | 'failed' | 'coming_soon';
	const words: Record<StatusKind, string> = {
		in_library: 'In your library',
		downloading: 'Downloading',
		missing: 'Missing',
		requested: 'Requested',
		approved: 'Approved',
		turned_down: 'Turned down',
		failed: 'Failed',
		coming_soon: 'Coming soon'
	};
	const tones: Record<StatusKind, 'ok' | 'wait' | 'fail' | 'info' | 'idle'> = {
		in_library: 'ok',
		downloading: 'info',
		missing: 'wait',
		requested: 'idle',
		approved: 'ok',
		turned_down: 'idle',
		failed: 'fail',
		coming_soon: 'idle'
	};
	/** statusWord is the default word for a status; percent is added while downloading. */
	export function statusWord(kind: StatusKind, percent?: number): string {
		return kind === 'downloading' && percent !== undefined ? `${words[kind]} ${Math.round(percent)}%` : words[kind];
	}
</script>

<script lang="ts">
	// Dot plus a word, never colour alone (spec §2).
	let { kind, percent, label, on = 'surface' }: { kind: StatusKind; percent?: number; label?: string; on?: 'surface' | 'art' } = $props();
</script>

<span class="ui-pill {tones[kind]} {on}"><i aria-hidden="true"></i>{label ?? statusWord(kind, percent)}</span>

<style>
	.ui-pill { display: inline-flex; align-items: center; gap: 6px; height: 24px; padding: 0 10px 0 8px; border-radius: 999px; background: var(--surface-2); color: var(--text); font: var(--t-subhead); white-space: nowrap; font-variant-numeric: tabular-nums; }
	i { width: 7px; height: 7px; border-radius: 50%; background: var(--text-faint); flex: none; }
	.ok i { background: var(--ok); }
	.wait i { background: var(--wait); }
	.fail i { background: var(--fail); }
	.info i { background: var(--info); }
	.ok { color: var(--ok-text); }
	.fail { color: var(--fail-text); }
	.art { background: rgb(0 0 0 / 46%); color: #fff; backdrop-filter: blur(10px); -webkit-backdrop-filter: blur(10px); }
</style>
