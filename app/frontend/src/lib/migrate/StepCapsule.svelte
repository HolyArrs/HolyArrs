<script lang="ts">
	import StatusIcon, { type StatusKey } from '#lib/ui/StatusIcon.svelte';

	let {
		tone = 'soft',
		live = false,
		fill = 0,
		word,
		detail,
		status,
		progress = false,
		label,
		testid
	}: {
		tone?: 'ok' | 'go' | 'soft' | 'bad' | 'wait';
		live?: boolean;
		fill?: number;
		word: string;
		detail?: string;
		status?: StatusKey;
		progress?: boolean;
		label?: string;
		testid?: string;
	} = $props();
	const f = $derived(Math.max(0, Math.min(1, Number.isFinite(fill) ? fill : 0)));
</script>

<span
	class="st-cap st-{tone}"
	class:st-live={live}
	style:--st-fill={f}
	role={progress ? 'progressbar' : undefined}
	aria-label={progress ? (label ?? word) : undefined}
	aria-valuemin={progress ? 0 : undefined}
	aria-valuemax={progress ? 100 : undefined}
	aria-valuenow={progress ? Math.round(f * 100) : undefined}
	aria-valuetext={progress ? [word, detail].filter(Boolean).join(' · ') : undefined}
	data-testid={testid}
>
	<span class="st-meter" aria-hidden="true"></span>
	{#if status}<span class="st-i" aria-hidden="true"><StatusIcon {status} size={16} /></span>{/if}
	<span class="st-tx" title={detail ? `${word} · ${detail}` : undefined}><b>{word}</b>{#if detail}<span class="st-d">{detail}</span>{/if}</span>
</span>

<style>
	.st-cap {
		--st-h: 40px;
		--st-r: 12px;
		position: relative;
		overflow: hidden;
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 7px;
		min-width: 0;
		height: var(--st-h);
		padding: 0 12px;
		border-radius: var(--st-r);
		background: light-dark(color-mix(in srgb, var(--st-c) 8%, rgb(255 255 255 / 0.5)), color-mix(in srgb, var(--st-c) 9%, transparent));
		box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--st-c) 34%, transparent);
		color: var(--st-ink);
		font: var(--t-subhead);
		font-weight: 600;
		box-sizing: border-box;
	}
	.st-bad { --st-c: light-dark(#a3170c, #ff6b63); --st-ink: light-dark(#5e0e06, #ffb3ae); }
	.st-go { --st-c: light-dark(#1d5fd6, #4b8dff); --st-ink: light-dark(#072a6b, #b5d1ff); }
	.st-ok { --st-c: light-dark(#18843f, #34c26a); --st-ink: light-dark(#073d1b, #a6efc0); }
	.st-soft { --st-c: light-dark(#5a6070, #9aa0ad); --st-ink: var(--text); }
	.st-wait { --st-c: light-dark(#8a5a00, #ffb340); --st-ink: light-dark(#3d2800, #ffd99a); }
	.st-i { position: relative; display: inline-grid; place-items: center; flex: none; }
	.st-tx { position: relative; display: flex; flex-wrap: nowrap; align-items: baseline; justify-content: center; column-gap: 6px; min-width: 0; line-height: 1.15; text-align: center; }
	.st-tx > b { flex: none; font-weight: 700; }
	.st-d { min-width: 0; font-weight: 500; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.st-tx > .st-d::before { content: '·'; margin-right: 6px; opacity: 0.7; }
	.st-meter { position: absolute; inset: 0; overflow: hidden; background: light-dark(color-mix(in srgb, var(--st-c) 15%, transparent), color-mix(in srgb, var(--st-c) 30%, transparent)); transform: scaleX(var(--st-fill, 0)); transform-origin: left; transition: transform 600ms cubic-bezier(0.22, 1, 0.36, 1); }
	.st-soft .st-meter { background: light-dark(color-mix(in srgb, var(--st-c) 14%, transparent), color-mix(in srgb, var(--st-c) 22%, transparent)); }
	.st-live .st-meter::after { content: ''; position: absolute; inset: 0; background: linear-gradient(90deg, transparent, rgb(255 255 255 / 0.22), transparent) no-repeat; background-size: 40% 100%; animation: st-shim 2.2s ease-in-out infinite; }
	@keyframes st-shim { from { background-position: -60% 0; } to { background-position: 160% 0; } }
	@media (pointer: coarse) { .st-cap { --st-h: 44px; } }
	@media (max-width: 767px) {
		.st-cap { padding: 0 8px; gap: 5px; font: 600 12px/1.15 var(--font-ui); }
		.st-tx > .st-d { font-size: 12px; }
	}
	@media (prefers-reduced-motion: reduce) { .st-live .st-meter::after { display: none; } .st-meter { transition: none; } }
</style>
