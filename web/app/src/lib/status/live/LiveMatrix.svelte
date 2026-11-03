<script lang="ts">
	// View "Matrix": a dot-matrix board, like a station sign. A 5×7 dot font
	// scrolls the line that matters (what runs and how far, or what comes
	// next and in how long), and the bottom row of dots is the progress bar.
	import type { LiveJobs } from '../livejobs.svelte.ts';
	import { wait } from '../livejobs.svelte.ts';
	import type { StatusRow } from '../status.ts';

	let { lj, upcoming }: { lj: LiveJobs; upcoming: StatusRow[]; recent: StatusRow[] } = $props();

	// Fewer, bigger dots on a narrow plate so the letters stay readable.
	let bw = $state(600);
	const COLS = $derived(bw < 460 ? 44 : 78);
	// Each glyph is 7 rows of 5 dots.
	const F: Record<string, string[]> = {
		A: ['.###.', '#...#', '#...#', '#####', '#...#', '#...#', '#...#'],
		B: ['####.', '#...#', '#...#', '####.', '#...#', '#...#', '####.'],
		C: ['.###.', '#...#', '#....', '#....', '#....', '#...#', '.###.'],
		D: ['####.', '#...#', '#...#', '#...#', '#...#', '#...#', '####.'],
		E: ['#####', '#....', '#....', '####.', '#....', '#....', '#####'],
		F: ['#####', '#....', '#....', '####.', '#....', '#....', '#....'],
		G: ['.###.', '#...#', '#....', '#.###', '#...#', '#...#', '.###.'],
		H: ['#...#', '#...#', '#...#', '#####', '#...#', '#...#', '#...#'],
		I: ['.###.', '..#..', '..#..', '..#..', '..#..', '..#..', '.###.'],
		J: ['..###', '...#.', '...#.', '...#.', '...#.', '#..#.', '.##..'],
		K: ['#...#', '#..#.', '#.#..', '##...', '#.#..', '#..#.', '#...#'],
		L: ['#....', '#....', '#....', '#....', '#....', '#....', '#####'],
		M: ['#...#', '##.##', '#.#.#', '#.#.#', '#...#', '#...#', '#...#'],
		N: ['#...#', '#...#', '##..#', '#.#.#', '#..##', '#...#', '#...#'],
		O: ['.###.', '#...#', '#...#', '#...#', '#...#', '#...#', '.###.'],
		P: ['####.', '#...#', '#...#', '####.', '#....', '#....', '#....'],
		Q: ['.###.', '#...#', '#...#', '#...#', '#.#.#', '#..#.', '.##.#'],
		R: ['####.', '#...#', '#...#', '####.', '#.#..', '#..#.', '#...#'],
		S: ['.####', '#....', '#....', '.###.', '....#', '....#', '####.'],
		T: ['#####', '..#..', '..#..', '..#..', '..#..', '..#..', '..#..'],
		U: ['#...#', '#...#', '#...#', '#...#', '#...#', '#...#', '.###.'],
		V: ['#...#', '#...#', '#...#', '#...#', '#...#', '.#.#.', '..#..'],
		W: ['#...#', '#...#', '#...#', '#.#.#', '#.#.#', '##.##', '#...#'],
		X: ['#...#', '#...#', '.#.#.', '..#..', '.#.#.', '#...#', '#...#'],
		Y: ['#...#', '#...#', '.#.#.', '..#..', '..#..', '..#..', '..#..'],
		Z: ['#####', '....#', '...#.', '..#..', '.#...', '#....', '#####'],
		'0': ['.###.', '#...#', '#..##', '#.#.#', '##..#', '#...#', '.###.'],
		'1': ['..#..', '.##..', '..#..', '..#..', '..#..', '..#..', '.###.'],
		'2': ['.###.', '#...#', '....#', '...#.', '..#..', '.#...', '#####'],
		'3': ['#####', '...#.', '..#..', '...#.', '....#', '#...#', '.###.'],
		'4': ['...#.', '..##.', '.#.#.', '#..#.', '#####', '...#.', '...#.'],
		'5': ['#####', '#....', '####.', '....#', '....#', '#...#', '.###.'],
		'6': ['..##.', '.#...', '#....', '####.', '#...#', '#...#', '.###.'],
		'7': ['#####', '....#', '...#.', '..#..', '.#...', '.#...', '.#...'],
		'8': ['.###.', '#...#', '#...#', '.###.', '#...#', '#...#', '.###.'],
		'9': ['.###.', '#...#', '#...#', '.####', '....#', '...#.', '.##..'],
		':': ['.....', '..#..', '..#..', '.....', '..#..', '..#..', '.....'],
		'%': ['##...', '##..#', '...#.', '..#..', '.#...', '#..##', '...##'],
		'.': ['.....', '.....', '.....', '.....', '.....', '..#..', '..#..'],
		',': ['.....', '.....', '.....', '.....', '..#..', '..#..', '.#...'],
		'-': ['.....', '.....', '.....', '.###.', '.....', '.....', '.....'],
		'/': ['....#', '....#', '...#.', '..#..', '.#...', '#....', '#....'],
		'\'': ['..#..', '..#..', '.....', '.....', '.....', '.....', '.....'],
		'(': ['...#.', '..#..', '.#...', '.#...', '.#...', '..#..', '...#.'],
		')': ['.#...', '..#..', '...#.', '...#.', '...#.', '..#..', '.#...'],
		'+': ['.....', '..#..', '..#..', '#####', '..#..', '..#..', '.....'],
		'&': ['.##..', '#..#.', '#.#..', '.#...', '#.#.#', '#..#.', '.##.#'],
		'·': ['.....', '.....', '.....', '..#..', '.....', '.....', '.....'],
		'?': ['.###.', '#...#', '....#', '...#.', '..#..', '.....', '..#..'],
		' ': ['.....', '.....', '.....', '.....', '.....', '.....', '.....']
	};

	const job = $derived(lj.running[0]);
	const coming = $derived(lj.coming(upcoming, 2));
	const line = $derived.by(() => {
		if (job) {
			const pct = job.progress > 0 ? `  ${Math.round(job.progress * 100)}%` : '';
			return `${job.label}${pct}   ${job.note || 'working'}`;
		}
		if (!coming.length) return 'nothing planned';
		const a = coming[0], b = coming[1];
		const dueA = lj.due(a) <= 0 ? 'due now' : `in ${wait(lj.due(a))}`;
		return `next  ${a.name}  ${dueA}${b ? `   then  ${b.name}  ${lj.due(b) <= 0 ? 'due now' : `in ${wait(lj.due(b))}`}` : ''}`;
	});
	const fill = $derived(job ? job.progress : coming[0] ? lj.fill(coming[0]) : 0);
	const tone = $derived(job ? 'live' : (coming[0]?.tone ?? 'idle'));

	// The line as columns of dots, with a one-column gap between glyphs and a
	// run of blank columns at the end so the loop breathes.
	const strip = $derived.by(() => {
		const cols: boolean[][] = [];
		// Accents are dropped (É reads as E); any other sign the font lacks shows as "?" so no letter goes missing.
		for (const ch of line.normalize('NFD').replace(/\p{M}/gu, '').toUpperCase()) {
			const g = F[ch] ?? (/\s/.test(ch) ? F[' '] : F['?']);
			for (let c = 0; c < 5; c++) cols.push(g.map((row) => row[c] === '#'));
			cols.push(Array(7).fill(false));
		}
		for (let i = 0; i < 12; i++) cols.push(Array(7).fill(false));
		return cols;
	});
	const fits = $derived(strip.length - 12 <= COLS);

	let step = $state(0);
	// The board scrolls only when the line does not fit. It can be paused, and
	// starts paused under reduced motion; while still, the caption shows the whole line.
	let reduce = $state(false);
	let paused = $state<boolean | null>(null);
	$effect(() => {
		if (typeof matchMedia !== 'function') return;
		const q = matchMedia('(prefers-reduced-motion: reduce)');
		reduce = q.matches;
		const on = () => (reduce = q.matches);
		q.addEventListener('change', on);
		return () => q.removeEventListener('change', on);
	});
	const still = $derived(paused ?? reduce);
	const moving = $derived(!fits && !still);
	$effect(() => {
		if (!moving) return;
		const i = setInterval(() => (step += 1), 110);
		return () => clearInterval(i);
	});
	// The window of columns on the board: the whole line when it fits, else a loop.
	const shown = $derived.by(() => {
		if (fits) {
			const pad = Math.floor((COLS - (strip.length - 12)) / 2);
			return Array.from({ length: COLS }, (_, c) => strip[c - pad] ?? Array(7).fill(false));
		}
		const n = strip.length;
		return Array.from({ length: COLS }, (_, c) => strip[(step + c) % n]);
	});
	const lit = $derived(Math.round(fill * COLS));
</script>

<div class="mx" data-tone={tone}>
	<div class="board" role="img" aria-label={line} bind:clientWidth={bw}>
		<div class="dots" style:--cols={COLS}>
			{#each Array(7) as _, r (r)}
				{#each shown as col, c (c)}
					<i class:on={col[r]}></i>
				{/each}
			{/each}
			{#each Array(COLS) as _, c (c)}
				<i class="bar" class:on={c < lit}></i>
			{/each}
		</div>
	</div>
	{#if !fits}
		<button type="button" class="ps" aria-pressed={still} onclick={() => (paused = !still)}>{still ? 'Play' : 'Pause'}</button>
	{/if}
	<p class="cap" class:full={!fits && still}>{!fits && still ? line : job ? `${job.label} · ${job.progress > 0 ? Math.round(job.progress * 100) + '%' : 'running'}` : coming[0] ? `${coming[0].name} · ${lj.due(coming[0]) <= 0 ? 'due' : wait(lj.due(coming[0]))}` : 'Nothing planned'}</p>
</div>

<style>
	.mx { display: flex; flex-direction: column; justify-content: center; min-height: 100%; min-width: 0; --lit: var(--text); }
	.mx[data-tone='live'] { --lit: var(--accent, var(--text)); }
	.mx[data-tone='ok'] { --lit: var(--ok); }
	.mx[data-tone='fail'] { --lit: var(--fail); }
	.mx[data-tone='wait'] { --lit: var(--wait); }
	.board { padding: 14px 12px; border-radius: 12px; background: color-mix(in srgb, var(--text) 5%, transparent); box-shadow: inset 0 1px 6px rgb(0 0 0 / 0.25), inset 0 0 0 1px color-mix(in srgb, var(--text) 8%, transparent); }
	.dots { display: grid; grid-template-columns: repeat(var(--cols), 1fr); gap: 1.5px; align-items: center; justify-items: center; }
	.dots i { display: block; width: 100%; aspect-ratio: 1; border-radius: 50%; background: color-mix(in srgb, var(--text) 7%, transparent); transition: background 0.12s linear, box-shadow 0.12s linear; }
	.dots i.on { background: var(--lit); box-shadow: 0 0 5px color-mix(in srgb, var(--lit) 60%, transparent); }
	.dots i.bar { margin-top: 6px; border-radius: 1px; aspect-ratio: 1 / 0.6; }
	.cap { margin: 10px 0 0; text-align: center; font: 500 12px/1.3 var(--font-mono); color: var(--text-muted); font-variant-numeric: tabular-nums; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.cap.full { white-space: normal; color: var(--text); }
	.ps { align-self: flex-end; margin: 8px 0 -4px; height: 28px; padding: 0 12px; border: 1px solid var(--line); border-radius: var(--r-pill); background: none; color: var(--text-muted); font: 500 12px/1 var(--font-ui); cursor: pointer; }
	.ps:hover { color: var(--text); }
	.ps:focus-visible { outline: none; box-shadow: var(--focus-ring); }
</style>
