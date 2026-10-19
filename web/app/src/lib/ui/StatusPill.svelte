<script lang="ts" module>
	import { statusLabel } from '../title-status.ts';
	export type StatusKind = 'in_library' | 'downloading' | 'missing' | 'requested' | 'approved' | 'turned_down' | 'failed' | 'coming_soon';
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
	/** statusWord is the default (staff) word for a status, from the one status vocabulary in title-status.ts. */
	export function statusWord(kind: StatusKind, percent?: number): string {
		return statusLabel(kind, 'staff', { percent });
	}
</script>

<script lang="ts">
	// Dot plus a word, never colour alone (spec §2).
	// Over art it becomes the shared glass pill with a line glyph instead of the dot.
	import ArtPill from './ArtPill.svelte';
	import type { IconName } from '../Icon.svelte';
	let { kind, percent, label, on = 'surface', testid }: { kind: StatusKind; percent?: number; label?: string; on?: 'surface' | 'art'; testid?: string } = $props();
	const glyphs: Record<StatusKind, IconName> = {
		in_library: 'owned',
		downloading: 'download',
		missing: 'partOwned',
		requested: 'waiting',
		approved: 'waiting',
		turned_down: 'declined',
		failed: 'alert',
		coming_soon: 'calendarDay'
	};
	const artTone = { ok: 'ok', wait: 'wait', fail: 'fail', info: 'info', idle: 'plain' } as const;
</script>

{#if on === 'art'}
	<ArtPill text={label ?? statusWord(kind, percent)} icon={glyphs[kind]} tone={artTone[tones[kind]]} percent={kind === 'downloading' ? percent : undefined} {testid} />
{:else}
	<span class="ui-pill {tones[kind]} {on}" data-testid={testid}><i aria-hidden="true"></i>{label ?? statusWord(kind, percent)}</span>
{/if}

<style>
	.ui-pill { display: inline-flex; align-items: center; gap: 6px; height: 24px; padding: 0 10px 0 8px; border-radius: 999px; background: var(--surface-2); color: var(--text); font: var(--t-subhead); white-space: nowrap; font-variant-numeric: tabular-nums; }
	i { width: 7px; height: 7px; border-radius: 50%; background: var(--text-faint); flex: none; }
	.ok i { background: var(--ok); }
	.wait i { background: var(--wait); }
	.fail i { background: var(--fail); }
	.info i { background: var(--info); }
	.ok { color: var(--ok-text); }
	.fail { color: var(--fail-text); }
</style>
