<script lang="ts">
	import type { RecentPerson } from './recent-people.ts';

	let { p, size = 36, plus = false }: { p?: RecentPerson; size?: number; plus?: boolean } = $props();

	const hue = (n: string) => [...n].reduce((a, c) => (a * 31 + c.charCodeAt(0)) % 360, 7);
</script>

{#if plus}
	<span class="face plus" aria-hidden="true">+</span>
{:else if p}
	<span class="face" style="--s: {size}px; --h: {hue(p.name)}" aria-hidden="true">
		{p.name.slice(0, 1).toUpperCase()}
		{#if p.pic}<img src={p.pic} alt="" />{/if}
		{#if p.way !== 'password'}
			<span class="badge">
				{#if p.way === 'plex'}
					<svg viewBox="0 0 24 24"><rect width="24" height="24" rx="6" fill="#1f1f1f" /><path d="M8 5h4.6l5 7-5 7H8l5-7z" fill="#E5A00D" /></svg>
				{:else if p.way === 'jellyfin'}
					<svg viewBox="0 0 24 24"><defs><linearGradient id="jg-face" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#AA5CC3" /><stop offset="1" stop-color="#00A4DC" /></linearGradient></defs><path d="M12 3c-2 0-9 12.4-8 14.4S20 19.4 21 17.4 14 3 12 3zm0 6c1 0 4.4 6 4 7s-7.6 1-8 0S11 9 12 9z" fill="url(#jg-face)" /></svg>
				{:else if p.way === 'emby'}
					<svg viewBox="0 0 24 24"><path d="M12 2l9 5v10l-9 5-9-5V7z" fill="#52B54B" /><path d="M10 8.5v7l6-3.5z" fill="#fff" /></svg>
				{:else}
					<svg viewBox="0 0 24 24" fill="none" stroke="#f5f5f7" stroke-width="1.8" stroke-linecap="round"><circle cx="8" cy="12" r="4" /><path d="M12 12h9M18 12v3M21 12v2" /></svg>
				{/if}
			</span>
		{/if}
	</span>
{/if}

<style>
	:global(.light .who) .face { box-shadow: 0 0 0 3px rgb(255 255 255 / 0.85), 0 10px 28px rgb(0 0 0 / 0.18); }
	:global(.light) .face.plus { background: rgb(0 0 0 / 0.05); border-color: rgb(0 0 0 / 0.25); color: rgb(0 0 0 / 0.6); }
	/* Light badges: a white disc, not a heavy black one. */
	:global(.light) .badge { background: #fff; box-shadow: 0 0 0 2px #fff; }
	:global(.light) .badge svg[stroke='#f5f5f7'] { stroke: #1d1d1f; }
	.face {
		position: relative;
		flex: none;
		display: inline-grid;
		place-items: center;
		width: var(--s, 36px);
		height: var(--s, 36px);
		border-radius: 50%;
		background: hsl(var(--h, 0) 45% 32%);
		color: #fff;
		font: 600 calc(var(--s, 36px) * 0.42)/1 var(--font-ui);
	}
	.face.plus { background: rgb(255 255 255 / 0.08); border: 1px dashed rgb(255 255 255 / 0.3); color: rgb(255 255 255 / 0.75); }
	.badge { position: absolute; right: -4px; bottom: -4px; width: 18px; height: 18px; border-radius: 50%; display: grid; place-items: center; background: #111; box-shadow: 0 0 0 2px #111; }
	.badge svg { width: 14px; height: 14px; }
	:global(.who) .face { box-shadow: 0 0 0 3px rgb(255 255 255 / 0.22), 0 10px 28px rgb(0 0 0 / 0.35); }
	:global(.who) .badge { right: 0; bottom: 0; width: 26px; height: 26px; }
	:global(.who) .badge svg { width: 18px; height: 18px; }
	.face img { position: absolute; inset: 0; width: 100%; height: 100%; border-radius: 50%; object-fit: cover; }
	/* Phones: the desktop card, floating low over full-bleed art with 24 px of art on every side; it rides above the keyboard (--kb) with a smooth lift. */
	@media (max-width: 600px) {
		:global(.who) .face { width: 72px; height: 72px; font-size: 30px; }
}
</style>
