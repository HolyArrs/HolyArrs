<!-- Crops a picked picture in place: the avatar circle itself shows it, drag (or arrow keys) to move, the slider, wheel, pinch or +/- to zoom. The square is then shrunk for upload (see shrink()). -->
<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { onMount } from 'svelte';

	let { file, size = 180, oncancel, ondone, onfail }: { file: File; size?: number; oncancel: () => void; ondone: (blob: Blob, url: string) => void; onfail?: () => void } = $props();
	let frame = $state<HTMLElement>();
	const hintId = $props.id();

	// The circle's size on screen: the avatar's own size, so nothing moves.
	const VIEW = $derived(size);
	let img = $state<HTMLImageElement | null>(null);
	let url = '';
	let zoom = $state(1);
	// The picture's centre offset from the frame's centre, in screen pixels.
	let x = $state(0);
	let y = $state(0);
	let busy = $state(false);
	// The scale that makes the short side fill the frame at zoom 1.
	const base = $derived(img ? VIEW / Math.min(img.naturalWidth, img.naturalHeight) : 1);
	const w = $derived(img ? img.naturalWidth * base * zoom : VIEW);
	const h = $derived(img ? img.naturalHeight * base * zoom : VIEW);

	// Keeps the frame covered: the picture never slides off an edge.
	function clamp() {
		const mx = Math.max(0, (w - VIEW) / 2);
		const my = Math.max(0, (h - VIEW) / 2);
		x = Math.min(mx, Math.max(-mx, x));
		y = Math.min(my, Math.max(-my, y));
	}
	function setZoom(z: number) {
		zoom = Math.min(4, Math.max(1, z));
		clamp();
	}

	onMount(() => {
		url = URL.createObjectURL(file);
		const i = new Image();
		i.onload = () => (img = i);
		// A file that won't decode: close the crop with a message instead of an empty circle.
		i.onerror = () => (onfail ?? oncancel)();
		i.src = url;
		// Focus stays with the photo, so keyboard users can move it straight away.
		frame?.focus({ preventScroll: true });
		return () => URL.revokeObjectURL(url);
	});

	// Drag with one pointer; pinch with two.
	const pts = new Map<number, { x: number; y: number }>();
	let pinch = 0;
	function down(e: PointerEvent) {
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
		pts.set(e.pointerId, { x: e.clientX, y: e.clientY });
		if (pts.size === 2) pinch = spread();
	}
	function spread() {
		const [a, b] = [...pts.values()];
		return Math.hypot(a.x - b.x, a.y - b.y);
	}
	function move(e: PointerEvent) {
		const p = pts.get(e.pointerId);
		if (!p) return;
		if (pts.size === 1) {
			x += e.clientX - p.x;
			y += e.clientY - p.y;
		}
		pts.set(e.pointerId, { x: e.clientX, y: e.clientY });
		if (pts.size === 2 && pinch) {
			const d = spread();
			setZoom((zoom * d) / pinch);
			pinch = d;
		}
		clamp();
	}
	function up(e: PointerEvent) {
		pts.delete(e.pointerId);
		pinch = 0;
	}
	function wheel(e: WheelEvent) {
		e.preventDefault();
		setZoom(zoom * (e.deltaY < 0 ? 1.08 : 1 / 1.08));
	}
	function key(e: KeyboardEvent) {
		const step = e.shiftKey ? 20 : 6;
		const moves: Record<string, [number, number]> = { ArrowLeft: [step, 0], ArrowRight: [-step, 0], ArrowUp: [0, step], ArrowDown: [0, -step] };
		if (moves[e.key]) {
			x += moves[e.key][0];
			y += moves[e.key][1];
			clamp();
		} else if (e.key === '+' || e.key === '=') setZoom(zoom * 1.1);
		else if (e.key === '-' || e.key === '_') setZoom(zoom / 1.1);
		else return;
		e.preventDefault();
	}

	// The square under the frame, drawn at side px and encoded.
	function encode(side: number, q: number): Promise<Blob | null> {
		const c = document.createElement('canvas');
		c.width = c.height = side;
		const g = c.getContext('2d');
		if (!g || !img) return Promise.resolve(null);
		g.imageSmoothingQuality = 'high';
		const k = side / VIEW;
		g.drawImage(img, (VIEW / 2 - w / 2 + x) * k, (VIEW / 2 - h / 2 + y) * k, w * k, h * k);
		return new Promise((ok) =>
			c.toBlob((b) => (b && b.type === 'image/webp' ? ok(b) : c.toBlob((j) => ok(j), 'image/jpeg', q)), 'image/webp', q)
		);
	}
	// 512 px WebP at 0.85 aims at 30-80 KB; a bigger result steps the quality down to 0.75, then the size down to 256 px, never further.
	async function shrink(): Promise<Blob | null> {
		let best: Blob | null = null;
		for (const side of [512, 384, 256]) {
			for (const q of [0.85, 0.8, 0.75]) {
				best = await encode(side, q);
				if (!best || best.size <= 80 * 1024) return best;
			}
		}
		return best;
	}
	async function use() {
		busy = true;
		const b = await shrink();
		busy = false;
		if (b) ondone(b, URL.createObjectURL(b));
	}
</script>

<div class="crop" data-testid="avatar-crop">
	<!-- A pan area moved by drag and arrow keys (zoom has its own slider), so it takes focus and keys. -->
	<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
	<div
		class="frame"
		bind:this={frame}
		role="application"
		tabindex="0"
		aria-label={t('welcome.account.cropMove')}
		aria-describedby="{hintId}-hint"
		style:width="{VIEW}px"
		style:height="{VIEW}px"
		onpointerdown={down}
		onpointermove={move}
		onpointerup={up}
		onpointercancel={up}
		onwheel={wheel}
		onkeydown={key}
	>
		{#if img}<img src={img.src} alt="" draggable="false" style:width="{w}px" style:height="{h}px" style:transform="translate({x - w / 2 + VIEW / 2}px, {y - h / 2 + VIEW / 2}px)" />{/if}
	</div>
	<span class="sr-only" id="{hintId}-hint">{t('welcome.account.cropHint')}</span>
	<div class="under">
		<input class="zoom" type="range" min="1" max="4" step="0.01" value={zoom} oninput={(e) => setZoom(Number(e.currentTarget.value))} aria-label={t('welcome.account.cropZoom')} title={t('welcome.account.cropHint')} />
		<div class="row">
			<button class="mini" type="button" disabled={busy} onclick={oncancel}>{t('welcome.link.cancel')}</button>
			<button class="mini go" type="button" disabled={busy || !img} onclick={use} data-testid="crop-use">{t('welcome.account.cropUse')}</button>
		</div>
	</div>
</div>

<style>
	.crop { display: flex; flex-direction: column; align-items: center; gap: 10px; }
	.frame { position: relative; overflow: hidden; border-radius: 50%; background: rgb(255 255 255 / 0.08); border: 1px solid rgb(255 255 255 / 0.16); cursor: grab; touch-action: none; user-select: none; flex: none; }
	.frame:active { cursor: grabbing; }
	.frame:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.frame img { position: absolute; left: 0; top: 0; max-width: none; pointer-events: none; }
	/* The controls fill the fixed space the hint uses (see .under in SigninLoginForm). */
	.under { display: flex; flex-direction: column; align-items: center; gap: 6px; width: 100%; }
	.zoom { width: 100%; max-width: 180px; margin: 0; accent-color: var(--accent); }
	.row { display: flex; gap: 8px; justify-content: center; }
	.mini { min-height: 28px; padding: 0 12px; border-radius: var(--r-btn); border: 1px solid rgb(255 255 255 / 0.18); background: transparent; color: inherit; font: 500 13px/1 var(--font-ui); cursor: pointer; }
	.mini.go { background: var(--accent); border-color: transparent; color: var(--on-accent, #fff); }
	.mini:disabled { opacity: 0.5; cursor: default; }
	@media (hover: hover) { .mini:not(.go):not(:disabled):hover { background: rgb(255 255 255 / 0.08); } :global(.light) .mini:not(.go):not(:disabled):hover { background: rgb(0 0 0 / 0.05); } }
	@media (pointer: coarse) { .mini { min-height: 44px; padding: 0 16px; } }
	:global(.light) .frame { border-color: rgb(0 0 0 / 0.12); background: rgb(0 0 0 / 0.05); }
	:global(.light) .mini:not(.go) { border-color: rgb(0 0 0 / 0.18); }
	.mini:focus-visible { outline: none; box-shadow: var(--focus-ring); }
</style>
