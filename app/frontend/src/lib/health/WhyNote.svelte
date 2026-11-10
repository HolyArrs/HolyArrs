<script lang="ts">
	import Words from '#lib/ui/Words.svelte';
	// Retro about window: types its lines, slams tags in, and sends runners across the title bar.
	import silkscreen from '@fontsource/silkscreen/files/silkscreen-latin-400-normal.woff2?url';
	import { tick } from 'svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { to } from '../core/paths.ts';

	// The pixel font ships with the app and loads once, so the window needs no network.
	if (typeof document !== 'undefined' && typeof FontFace !== 'undefined' && ![...document.fonts].some((f) => f.family === 'Silkscreen')) {
		const face = new FontFace('Silkscreen', `url(${to(silkscreen)}) format('woff2')`, { display: 'swap' });
		document.fonts.add(face);
		void face.load().catch(() => {});
	}

	// The Support block stays hidden until this holds a real link.
	const SUPPORT_URL = '';
	// Run-cycle frames for the two runners.
	const luisF = [0, 1, 2, 1].map((i) => to(`/why/coderluii-run-${i}.png`));
	const dogF = [0, 1, 2, 3, 4, 5].map((i) => to(`/why/peanut-run-${i}.png`));

	let dlg = $state<HTMLDialogElement>();
	let closing = $state(false);
	let playing = $state(false);
	let title = $state('');

	// Plain variables: the DOM-driven animation reads them, the markup does not.
	let timers: ReturnType<typeof setTimeout>[] = [];
	let run = 0;
	const originals = new Map<Element, string>();
	let scene = 0;
	let raf = 0;
	let sceneTimer: ReturnType<typeof setTimeout> | undefined;

	export async function open() {
		closing = false;
		if (!dlg) return;
		if (!dlg.open) dlg.showModal();
		await tick();
		resetScene();
		sceneLater();
		void play();
	}
	function close() {
		run++;
		timers.forEach(clearTimeout);
		timers = [];
		resetScene();
		if (reduce()) return dlg?.close();
		closing = true;
		setTimeout(() => { dlg?.close(); closing = false; }, 250);
	}
	function onScreenClick(e: MouseEvent) {
		if (playing && !(e.target as HTMLElement).closest('a')) skip();
	}
	function onKey(e: KeyboardEvent) {
		if (e.key === ' ' && playing) { e.preventDefault(); skip(); }
	}

	// A click or space ends the typing early.
	function skip() {
		run++;
		timers.forEach(clearTimeout);
		timers = [];
		finish();
		resetScene();
		sceneLater();
	}

	// Motion is skipped for people who ask for less of it.
	const reduce = () => typeof matchMedia !== 'undefined' && matchMedia('(prefers-reduced-motion: reduce)').matches;
	// Every timer goes through later so close and skip can cancel them all.
	const later = (ms: number, f: () => void) => timers.push(setTimeout(f, ms));
	const wait = (ms: number) => new Promise<void>((r) => later(ms, r));

	// Removing and re-adding the class restarts the CSS animation.
	function setOn(el: Element) {
		el.classList.remove('on');
		void (el as HTMLElement).offsetWidth;
		el.classList.add('on');
	}
	// The window thuds a moment after a tag lands.
	function slam(el: Element | null) {
		if (!el || !dlg) return;
		setOn(el);
		const w = dlg.querySelector('.win');
		w?.classList.remove('thud');
		later(150, () => { void (w as HTMLElement | null)?.offsetWidth; w?.classList.add('thud'); });
	}
	// Letters are wrapped first and revealed later, so the layout never reflows while typing.
	async function typeIn(el: Element, perChar: number, id: number) {
		const nodes: Text[] = [];
		const walk = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
		while (walk.nextNode()) nodes.push(walk.currentNode as Text);
		const chars: HTMLElement[] = [];
		for (const n of nodes) {
			const frag = document.createDocumentFragment();
			for (const ch of n.textContent ?? '') { const sp = document.createElement('span'); sp.className = 'ch'; sp.textContent = ch; frag.append(sp); chars.push(sp); }
			n.replaceWith(frag);
		}
		el.classList.add('on');
		const cur = document.createElement('span');
		cur.className = 'tcur';
		for (let i = 0; i < chars.length; i++) {
			if (id !== run) return;
			chars[i].classList.add('v');
			chars[i].after(cur);
			if (i % 2 === 1) await wait(perChar * 2);
		}
		cur.remove();
	}
	// Jumps to the finished page: all text shown, no animation.
	function finish() {
		if (!dlg) return;
		title = t('health.why.title');
		dlg.querySelectorAll('.typed').forEach((el) => { const o = originals.get(el); if (o !== undefined) el.innerHTML = o; });
		dlg.querySelectorAll('.stk, .typed').forEach((el) => { el.classList.add('on'); (el as HTMLElement).style.animation = 'none'; });
		playing = false;
	}
	async function play() {
		if (!dlg) return;
		const id = ++run;
		timers.forEach(clearTimeout);
		timers = [];
		dlg.querySelectorAll('.typed').forEach((el) => { if (!originals.has(el)) originals.set(el, el.innerHTML); el.innerHTML = originals.get(el) ?? ''; el.classList.remove('on'); });
		dlg.querySelectorAll('.stk, .typed').forEach((el) => { el.classList.remove('on'); (el as HTMLElement).style.animation = ''; });
		if (reduce()) return finish();
		playing = true;
		title = '';
		for (const c of t('health.why.title')) { await wait(80); if (id !== run) return; title += c; }
		slam(dlg.querySelector('.photo'));
		await wait(260);
		for (const beat of dlg.querySelectorAll('.story > section')) {
			if (id !== run) return;
			if (beat.classList.contains('thanks')) { slam(beat); await wait(260); }
			slam(beat.querySelector('.wtag'));
			await wait(420);
			for (const el of beat.querySelectorAll('.typed, .wchip, .wone')) {
				if (id !== run) return;
				if (el.classList.contains('typed')) await typeIn(el, el.classList.contains('big') ? 42 : el.tagName === 'LI' ? 22 : 24, id);
				else { slam(el); await wait(200); }
				await wait(320);
			}
			await wait(600);
		}
		if (id !== run) return;
		const sp = dlg.querySelector('.who .support');
		if (sp) { slam(sp); later(260, () => slam(sp.querySelector('a'))); }
		playing = false;
	}

	// Puts the runners back off screen before a new run or on close.
	function resetScene() {
		scene++;
		cancelAnimationFrame(raf);
		clearTimeout(sceneTimer);
		const r = dlg?.querySelector('.run');
		if (!r) return;
		r.querySelectorAll<HTMLElement>('.runner').forEach((el) => { el.style.opacity = '0'; el.style.clipPath = 'none'; el.style.transform = 'none'; });
		r.querySelectorAll('.dust, .streak').forEach((p) => p.remove());
		const portal = r.querySelector('.portal');
		if (portal) portal.className = 'portal';
		dlg?.querySelectorAll('.wbar .wx, .wbar .wskip').forEach((b) => b.classList.remove('lift', 'bonk'));
	}
	// Own timer, so cancelling the letters does not cancel the runners.
	function sceneLater(ms = 5000) {
		clearTimeout(sceneTimer);
		const sc = scene;
		sceneTimer = setTimeout(() => { if (sc === scene) runScene(); }, ms);
	}
	// Runners enter at the portal, run to the button, and bonk it.
	function runScene() {
		if (reduce() || !dlg?.open) return;
		const id = ++scene;
		const bar = dlg.querySelector('.wbar') as HTMLElement;
		const r = bar.querySelector('.run') as HTMLElement;
		const portal = r.querySelector('.portal') as HTMLElement;
		const sk = bar.querySelector('.wskip');
		const x = (sk && playing ? sk : bar.querySelector('.wx')) as HTMLElement;
		const luis = r.querySelector('.luis') as HTMLImageElement;
		const dog = r.querySelector('.dog') as HTMLImageElement;
		const br = bar.getBoundingClientRect();
		const caret = (bar.querySelector('.caret') as HTMLElement).getBoundingClientRect();
		const xr = x.getBoundingClientRect();
		const start = caret.right - br.left + 10, stop = xr.left - br.left + 6, end = xr.right - br.left - 8;
		portal.style.left = `${start}px`;
		portal.className = 'portal open';
		let t0 = 0, lifted = false, lastStep = -1;
		const puff = (at: number) => { if (at > stop - 8) return; const p = document.createElement('i'); p.className = 'dust'; p.style.left = `${at}px`; r.append(p); setTimeout(() => p.remove(), 450); };
		const streak = (at: number, y: number) => { if (at > stop - 14) return; const p = document.createElement('i'); p.className = 'streak'; p.style.left = `${at - 16}px`; p.style.bottom = `${y}px`; r.append(p); setTimeout(() => p.remove(), 320); };
		// Pixels per second.
		const speed = 95;
		const step = (now: number) => {
			if (id !== scene || !dlg?.open) return;
			if (!t0) t0 = now;
			const t = (now - t0) / 1000;
			if (t < 0.45) { raf = requestAnimationFrame(step); return; }
			const lf = Math.floor(t * 9) % 4, df = Math.floor(t * 13 + 3) % 6;
			luis.src = luisF[lf];
			dog.src = dogF[df];
			const lb = lf === 1 || lf === 3 ? -2 : 0, db = df === 1 || df === 4 ? -2 : 0;
			const lx = start - 4 + (t - 0.45) * speed, dx = Math.max(start - 4, lx - 32);
			const stride = Math.floor(t * 4.5);
			if (stride !== lastStep) { lastStep = stride; puff(lx + 10); puff(dx + 6); streak(lx - 4, 16); streak(dx - 6, 8); }
			luis.style.opacity = dog.style.opacity = '1';
			luis.style.transform = `translate(${lx}px, ${lb}px) rotate(4deg)`;
			dog.style.transform = `translate(${dx}px, ${db}px) rotate(3deg)`;
			if (t > 1.1 && portal.classList.contains('open')) portal.className = 'portal shut';
			if (!lifted && lx + 30 >= stop) { lifted = true; x.classList.add('lift'); }
			const clip = stop + 4;
			luis.style.clipPath = lx + 40 > clip ? `inset(0 ${Math.min(40, lx + 40 - clip)}px 0 0)` : 'none';
			dog.style.clipPath = dx + 26 > clip ? `inset(0 ${Math.min(26, dx + 26 - clip)}px 0 0)` : 'none';
			if (dx > end) {
				luis.style.opacity = dog.style.opacity = '0';
				x.classList.remove('lift');
				x.classList.add('bonk');
				setTimeout(() => x.classList.remove('bonk'), 300);
				sceneLater(11000);
				return;
			}
			raf = requestAnimationFrame(step);
		};
		raf = requestAnimationFrame(step);
	}
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
<dialog bind:this={dlg} class="why" class:closing aria-labelledby="why-h" oncancel={(e) => { e.preventDefault(); close(); }} onclick={(e) => { if (e.target === dlg) close(); }} onkeydown={onKey} data-testid="why-note">
	<div class="win">
		<div class="wbar">
			<span class="wdots" aria-hidden="true"><i></i><i></i><i></i></span>
			<h2 id="why-h"><span aria-hidden="true">{title}</span><span class="sr-only">{t('health.why.title')}</span><span class="caret" aria-hidden="true"></span></h2>
			{#if playing}<button class="wskip" type="button" onclick={skip}>{t('health.why.skip')} <span aria-hidden="true">&gt;&gt;</span></button>{/if}
			<button class="wx" type="button" aria-label={t('health.why.close')} onclick={close}>X</button>
			<div class="run" aria-hidden="true"><i class="portal"></i><img class="runner luis" alt="" src={luisF[0]} /><img class="runner dog" alt="" src={dogF[0]} /></div>
		</div>
		<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
		<div class="screen" class:playing onclick={onScreenClick}>
			<aside class="who">
				<div class="photo stk" style="--r0:-14deg;--r1:-2deg"><img src={to('/why/luis.webp')} alt={t('health.why.photoAlt')} /><span class="scan" aria-hidden="true"></span></div>
				<div class="name"><b>Luis</b><small>{t('health.why.made')}</small><a href="https://coderluii.dev" target="_blank" rel="noopener">coderluii.dev</a></div>
				{#if SUPPORT_URL}<div class="support stk" style="--r0:6deg"><p>{t('health.why.saves')}</p><a href={SUPPORT_URL} target="_blank" rel="noopener" class="stk" style="--r0:-16deg"><span aria-hidden="true">♥</span> {t('health.why.support')}</a></div>{/if}
			</aside>
			<div class="story">
				<section>
					<span class="wtag stk">{t('health.why.then')}</span>
					<p class="big typed"><Words key="health.why.years" /></p>
					<div class="wapps">
						<span class="wchip stk" style="--c:#4f8cff"><i></i>{t('health.why.shows')}</span>
						<span class="wchip stk" style="--c:#f2b33d"><i></i>{t('health.why.movies')}</span>
						<span class="wchip stk" style="--c:#2fbf71"><i></i>{t('health.why.music')}</span>
						<span class="wchip stk" style="--c:#a78bfa"><i></i>{t('health.why.subtitles')}</span>
						<span class="wchip stk" style="--c:#38bdf8"><i></i>{t('health.why.sources')}</span>
						<span class="wchip stk" style="--c:#fb7185"><i></i>{t('health.why.requests')}</span>
					</div>
					<p class="say typed">{t('health.why.loved')}</p>
				</section>
				<section>
					<span class="wtag stk">{t('health.why.now')}</span>
					<p class="big typed">{t('health.why.built')}</p>
					<div class="onerow"><span class="wone stk">{t('health.why.one')}</span></div>
					<p class="say typed">{t('health.why.asks')}</p>
				</section>
				<section>
					<span class="wtag stk">{t('health.why.building')}</span>
					<ul class="vision">
						<li class="typed">{t('health.why.v1')}</li>
						<li class="typed">{t('health.why.v2')}</li>
						<li class="typed">{t('health.why.v3')}</li>
						<li class="typed">{t('health.why.v4')}</li>
					</ul>
				</section>
				<section class="thanks stk" style="--r0:-4deg">
					<span class="wtag tag-heart stk"><span class="heart" aria-hidden="true">{#each '01101101111111011111000111000001000' as c, i (i)}{#if c === '1'}<b></b>{:else}<s></s>{/if}{/each}</span>{t('health.why.thanks')}</span>
					<p class="big typed">{t('health.why.thanksBig')}</p>
					<p class="say typed">{t('health.why.thanksSay')}<span class="cur" aria-hidden="true"></span></p>
				</section>
			</div>
		</div>
	</div>
</dialog>

<style>
	.why { --coal: #191d20; --coal-2: #22272b; --cream: #f3efe5; --muted: #b6bec3; --red: #cf3025; --red-soft: #ff9384; --shade: #0b0d0f; --pixel: 'Silkscreen', var(--font-mono, ui-monospace), monospace;
		padding: 0; border: 0; background: none; color: var(--cream); width: min(820px, calc(100vw - 24px)); max-width: none; max-height: calc(100dvh - 24px); overflow: visible; }
	.why::backdrop { background: rgb(5 6 8 / 78%); backdrop-filter: blur(4px); }
	.win { display: flex; flex-direction: column; max-height: calc(100dvh - 32px); background: var(--coal); border: 3px solid var(--cream); box-shadow: 10px 10px 0 var(--red), 10px 10px 0 3px var(--shade); }
	.why[open] .win { animation: power 480ms cubic-bezier(.22, 1, .36, 1) both; }
	.why.closing .win { animation: power-off 260ms cubic-bezier(.4, 0, 1, 1) both; }
	.win:global(.thud) { animation: thud 180ms ease-out; }

	.wbar { position: relative; flex: none; display: flex; align-items: center; gap: 10px; padding: 8px 10px 8px 14px; background: var(--cream); color: var(--coal); }
	.wbar h2 { margin: 0; flex: 1; min-width: 0; font: 400 15px/1 var(--pixel); letter-spacing: .16em; text-transform: uppercase; padding: 4px 0; white-space: nowrap; }
	.caret { display: inline-block; width: .6em; height: 1em; background: var(--red); vertical-align: -2px; margin-left: 3px; animation: blink 1s steps(1) infinite; }
	.wdots { display: flex; gap: 6px; } .wdots i { width: 10px; height: 10px; background: var(--coal); } .wdots i:first-child { background: var(--red); }
	.wx, .wskip { all: unset; cursor: pointer; position: relative; z-index: 2; display: inline-flex; align-items: center; justify-content: center; gap: 6px; height: 30px; border: 2px solid var(--coal); background: var(--cream); color: var(--coal); font: 400 11px/1 var(--pixel); letter-spacing: .16em; text-transform: uppercase; transform-origin: 100% 100%; transition: transform 160ms cubic-bezier(.2, 1.4, .4, 1); }
	.wx { width: 30px; font-size: 14px; letter-spacing: 0; }
	.wskip { padding: 0 10px; }
	.wx:hover, .wskip:hover { background: var(--coal); color: var(--cream); }
	.wx:focus-visible, .wskip:focus-visible { outline: 2px solid var(--red); outline-offset: 2px; }
	.wbar :global(.lift) { transform: translateY(-5px) rotate(-14deg); box-shadow: 0 3px 0 var(--coal); }
	.wbar :global(.bonk) { animation: bonk 260ms ease-out; }
	@media (pointer: coarse) { .wx { width: 44px; height: 44px; } .wskip { height: 44px; } }

	.run { position: absolute; inset: 0; pointer-events: none; overflow: hidden; }
	.portal { position: absolute; bottom: 2px; width: 26px; height: 38px; transform: scaleY(0); transform-origin: bottom; background: radial-gradient(ellipse at 50% 55%, #fff 0 12%, var(--red-soft) 13% 30%, var(--red) 31% 55%, #5b1712 56% 70%, transparent 71%); filter: drop-shadow(0 0 6px rgb(207 48 37 / 70%)); }
	.portal:global(.open) { animation: portal-in 360ms steps(6) forwards, portal-spin 600ms steps(4) 360ms infinite; }
	.portal:global(.shut) { animation: portal-out 260ms steps(4) forwards; }
	.runner { position: absolute; bottom: 0; left: 0; image-rendering: pixelated; opacity: 0; will-change: transform; }
	.luis { width: 40px; height: 40px; }
	.dog { width: 26px; height: 24px; }
	.run :global(.dust) { position: absolute; bottom: 1px; width: 4px; height: 4px; background: #cfc8b8; box-shadow: 4px -2px 0 #ddd7c9, -3px -1px 0 #e5dfd2; animation: dust 420ms steps(4) forwards; }
	.run :global(.streak) { position: absolute; width: 16px; height: 2px; background: linear-gradient(90deg, transparent, rgb(25 29 32 / 25%)); animation: streak 300ms linear forwards; }

	.screen { position: relative; overflow: auto; scrollbar-width: none; display: grid; grid-template-columns: 196px minmax(0, 1fr); gap: 30px; padding: 24px 30px 22px; background: radial-gradient(120% 90% at 30% 0%, #2a3035 0%, var(--coal) 70%); }
	.screen::-webkit-scrollbar { display: none; }
	.screen.playing { cursor: pointer; }
	.screen::before { content: ''; position: absolute; inset: 0; pointer-events: none; background: repeating-linear-gradient(0deg, rgb(0 0 0 / 20%) 0 1px, transparent 1px 3px); z-index: 2; }
	.screen::after { content: ''; position: absolute; left: 0; right: 0; height: 140px; top: -140px; pointer-events: none; background: linear-gradient(180deg, transparent, rgb(243 239 229 / 5%), transparent); animation: sweep 7s linear infinite; z-index: 2; }

	.who { position: sticky; top: 0; align-self: start; display: grid; gap: 14px; }
	.photo { position: relative; width: 190px; aspect-ratio: 1; border: 3px solid var(--cream); box-shadow: 6px 6px 0 var(--red); background: #000; overflow: hidden; }
	.photo img { width: 100%; height: 100%; object-fit: cover; display: block; }
	.photo .scan { position: absolute; left: 0; right: 0; height: 30%; top: -30%; background: linear-gradient(180deg, transparent, rgb(207 48 37 / 28%), transparent); mix-blend-mode: screen; }
	.why[open] .photo .scan { animation: sweep 2.4s 600ms ease-in-out infinite; }
	.name b { display: block; font: 400 18px/1 var(--pixel); letter-spacing: .16em; text-transform: uppercase; padding: 3px 0; }
	.name small { display: block; margin-top: 6px; color: var(--muted); font-size: 13px; }
	.name a { display: inline-block; margin-top: 8px; padding: 2px 0; color: var(--red-soft); text-decoration: none; font: 400 11px/1 var(--pixel); letter-spacing: .16em; text-transform: uppercase; }
	.support { display: grid; gap: 10px; justify-items: start; margin-top: 6px; padding: 12px 14px; border: 2px solid rgb(243 239 229 / 18%); background: var(--coal-2); }
	.support p { margin: 0; color: color-mix(in srgb, var(--cream) 78%, transparent); font-size: 13px; line-height: 1.4; }
	.support a { display: inline-flex; align-items: center; gap: 10px; height: 40px; padding: 0 16px; background: var(--cream); color: var(--coal); border: 2px solid var(--coal); box-shadow: 4px 4px 0 var(--red); text-decoration: none; font: 400 12px/1 var(--pixel); letter-spacing: .16em; text-transform: uppercase; --r1: -2deg; }

	.story { position: relative; display: grid; gap: 18px; }
	.wtag { display: inline-flex; align-items: center; gap: 8px; margin-bottom: 8px; padding: 2px 0; font: 400 11px/1 var(--pixel); letter-spacing: .16em; text-transform: uppercase; color: var(--red-soft); }
	.wtag::before { content: ''; width: 8px; height: 8px; background: var(--red); }
	.tag-heart::before { display: none; }
	.heart { display: inline-grid; grid-template-columns: repeat(7, 3px); grid-auto-rows: 3px; margin-right: 2px; }
	.heart b { background: var(--red); }
	.big { margin: 0; font: 600 21px/1.3 var(--font-ui); letter-spacing: -.01em; color: var(--cream); text-wrap: balance; }
	.big :global(em) { font-style: normal; color: var(--red-soft); }
	.say { margin: 8px 0 0; font-size: 15px; line-height: 1.6; color: color-mix(in srgb, var(--cream) 78%, transparent); }
	.wapps { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 12px; }
	.wchip { display: inline-flex; align-items: center; gap: 7px; height: 32px; padding: 0 12px; border: 2px solid rgb(243 239 229 / 35%); background: var(--coal-2); font: 400 10.5px/1 var(--pixel); letter-spacing: .16em; text-transform: uppercase; color: var(--muted); transform: rotate(var(--r1, 0deg)); }
	.wchip i { width: 7px; height: 7px; background: var(--c, #888); }
	.wchip:nth-child(odd) { --r1: -2deg; } .wchip:nth-child(3n) { --r1: 3deg; translate: 0 3px; } .wchip:nth-child(even) { --r0: 12deg; }
	.onerow { margin-top: 10px; }
	.wone { display: inline-flex; align-items: center; height: 34px; padding: 0 16px; background: var(--red); color: var(--cream); border: 2px solid var(--cream); box-shadow: 4px 4px 0 var(--shade); font: 400 12px/1 var(--pixel); letter-spacing: .16em; text-transform: uppercase; --r1: -3deg; --r0: 14deg; transform: rotate(-3deg); }
	.vision { display: grid; gap: 8px; margin: 0; padding: 0; list-style: none; }
	.vision li { position: relative; padding-left: 20px; font-size: 15px; line-height: 1.55; color: color-mix(in srgb, var(--cream) 86%, transparent); }
	.vision li::before { content: ''; position: absolute; left: 2px; top: 8px; width: 8px; height: 8px; background: var(--red); box-shadow: 2px 2px 0 var(--shade); }
	.thanks { padding: 16px 20px; border: 2px dashed rgb(243 239 229 / 30%); background: rgb(207 48 37 / 8%); }
	.thanks .big { font-size: 18px; }
	.cur { display: inline-block; width: 8px; height: 15px; margin-left: 6px; vertical-align: -2px; background: var(--cream); animation: blink 1s steps(1) infinite; }

	/* Hidden until the script slams or types each part in. */
	.stk, .typed { visibility: hidden; }
	.stk:global(.on), .typed:global(.on) { visibility: visible; }
	.stk:global(.on) { animation: slam 380ms cubic-bezier(.2, 1.4, .4, 1) both; }
	.typed :global(.ch) { opacity: 0; }
	.typed :global(.ch.v) { opacity: 1; }
	.typed :global(.tcur) { display: inline-block; width: .5em; height: 1em; margin-left: 1px; margin-right: -.5em; vertical-align: -2px; background: var(--red-soft); }

	@keyframes power { 0%, 45% { transform: scale(1, .004); filter: brightness(3); } 100% { transform: none; filter: none; } }
	@keyframes power-off { 60% { transform: scale(1, .004); filter: brightness(3); } 100% { transform: scale(0, .004); } }
	@keyframes slam { 0% { opacity: 0; transform: scale(2.1) rotate(var(--r0, -10deg)); filter: blur(2px); } 55% { opacity: 1; transform: scale(.9) rotate(var(--r1, 0deg)); filter: none; } 100% { transform: scale(1) rotate(var(--r1, 0deg)); } }
	@keyframes thud { 30% { transform: translate(2px, 2px); } 60% { transform: translate(-1px, 0); } }
	@keyframes bonk { 30% { transform: translateX(3px) rotate(4deg); } 60% { transform: translateX(-1px); } }
	@keyframes portal-in { to { transform: scaleY(1); } }
	@keyframes portal-spin { 50% { filter: drop-shadow(0 0 10px rgb(255 147 132 / 90%)); transform: scaleY(1) scaleX(.85); } }
	@keyframes portal-out { from { transform: scaleY(1); } to { transform: scaleY(0) scaleX(1.4); } }
	@keyframes dust { to { opacity: 0; transform: translate(-10px, -6px) scale(1.6); } }
	@keyframes streak { to { opacity: 0; transform: translateX(-14px) scaleX(.4); } }
	@keyframes blink { 50% { opacity: 0; } }
	@keyframes sweep { to { top: 110%; } }

	@media (prefers-reduced-motion: reduce) {
		.why[open] .win, .why.closing .win, .screen::after, .caret, .cur, .why[open] .photo .scan { animation: none; }
		.run { display: none; }
		.stk, .typed { visibility: visible; animation: none !important; }
		.typed :global(.ch) { opacity: 1; }
	}
	@media (max-width: 640px) {
		.screen { grid-template-columns: 1fr; gap: 22px; padding: 22px 18px; }
		.who { position: static; grid-template-columns: 96px 1fr; align-items: center; }
		.photo { width: 96px; }
		.support { grid-column: 1 / -1; }
		.big { font-size: 19px; } .thanks .big { font-size: 17px; }
		.win { box-shadow: 6px 6px 0 var(--red), 6px 6px 0 3px var(--shade); }
	}
</style>
