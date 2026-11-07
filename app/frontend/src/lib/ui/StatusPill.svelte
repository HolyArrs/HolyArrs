<script lang="ts" module>
	// Small coloured pill that names a title or request state.
	import { statusLabel } from './title-status.ts';
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
	// Staff wording is used everywhere the pill shows its own text.
	export function statusWord(kind: StatusKind, percent?: number): string {
		return statusLabel(kind, 'staff', { percent });
	}
</script>

<script lang="ts">
	import ArtPill from './ArtPill.svelte';
	import type { IconName } from './Icon.svelte';
	let { kind, percent, label, on = 'surface', compact = false, testid }: { kind: StatusKind; percent?: number; label?: string; on?: 'surface' | 'art'; compact?: boolean; testid?: string } = $props();
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
	// the art pill has no "idle" tone, so idle maps to plain
	const artTone = { ok: 'ok', wait: 'wait', fail: 'fail', info: 'info', idle: 'plain' } as const;
</script>

<!-- over artwork the pill needs its own backing, so it uses ArtPill -->
{#if on === 'art'}
	<ArtPill text={label ?? statusWord(kind, percent)} icon={glyphs[kind]} tone={artTone[tones[kind]]} percent={kind === 'downloading' ? percent : undefined} {compact} label={compact ? (label ?? statusWord(kind, percent)) : undefined} {testid} />
{:else}
	<span class="ui-pill {tones[kind]} {on}" data-testid={testid}><i aria-hidden="true"></i><span class="t">{label ?? statusWord(kind, percent)}</span></span>
{/if}

<style>
	.ui-pill { display: inline-flex; align-items: center; gap: 6px; height: 24px; padding: 0 10px 0 8px; border-radius: 999px; background: var(--surface-2); color: var(--text); font: var(--t-subhead); white-space: nowrap; font-variant-numeric: tabular-nums; max-width: 100%; min-width: 0; }
	/* long labels truncate instead of widening the pill */
	.t { min-width: 0; overflow: hidden; text-overflow: ellipsis; }
	/* the dot carries the tone, the text stays neutral except ok and fail */
	i { width: 7px; height: 7px; border-radius: 50%; background: var(--text-faint); flex: none; }
	.ok i { background: var(--ok); }
	.wait i { background: var(--wait); }
	.fail i { background: var(--fail); }
	.info i { background: var(--info); }
	.ok { color: var(--ok-text); }
	.fail { color: var(--fail-text); }
</style>
