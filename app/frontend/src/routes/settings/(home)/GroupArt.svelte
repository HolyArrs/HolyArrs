<script lang="ts">
	// One Settings home card's picture: this Library's posters, releases coming down, the people on this install, the apps it links to, a rack with a heartbeat. Where the install has no posters or people yet, the card draws the same idea in line art: one line weight, one red detail. Everything is sized from the art box itself (container units), and hover moves only the whole box (the card does that), never a piece inside it.
	import { initials, type Person } from './pick.ts';
	import { coverFor } from '#lib/music/index.ts';
	import plex from 'simple-icons/icons/plex.svg?raw';
	import jellyfin from 'simple-icons/icons/jellyfin.svg?raw';
	import emby from 'simple-icons/icons/emby.svg?raw';
	import discord from 'simple-icons/icons/discord.svg?raw';
	import { to } from '#lib/core/paths.ts';

	let { group, posters = [], people = [] }: { group: string; posters?: string[]; people?: Person[] } = $props();

	const d = (svg: string) => / d="([^"]+)"/.exec(svg)?.[1] ?? '';
	const LOGOS = [
		{ k: 'plex', color: '#EBAF00', path: d(plex), x: '20%', y: '26%' },
		{ k: 'jellyfin', color: '#00A4DC', path: d(jellyfin), x: '80%', y: '26%' },
		{ k: 'emby', color: '#52B54B', path: d(emby), x: '20%', y: '74%' },
		{ k: 'discord', color: '#5865F2', path: d(discord), x: '80%', y: '74%' }
	];
	const TINTS = ['linear-gradient(160deg,#6b7d88,#3d4a52)', 'linear-gradient(160deg,#82709a,#4f4462)', 'linear-gradient(160deg,#94795a,#5f4b36)', 'linear-gradient(160deg,#5f7b97,#384d62)'];

	// Library: a tilted poster wall (twelve slots, repeated when the Library is small).
	const shelfP = $derived(posters.length >= 3 ? Array.from({ length: 12 }, (_, i) => posters[i % posters.length]) : []);
	const rel = $derived(posters.length >= 3 ? [posters[(posters.length > 7 ? 7 : 2) % posters.length], posters[(posters.length > 3 ? 3 : 1) % posters.length], posters[(posters.length > 4 ? 4 : 0) % posters.length]] : []);
	const crowd = $derived(people.length >= 2 ? people.slice(0, 5) : []);
	const more = $derived(people.length - crowd.length);

	// Line art pieces.
	const poster = (x: number, y: number, w: number, h: number, ac = false) =>
		`<g><rect class="o${ac ? ' ac' : ''}" x="${x}" y="${y}" width="${w}" height="${h}" rx="4"/><path class="${ac ? 'ac' : ''}" d="M${x + 8} ${y + h - 14} l${w * 0.28} -${h * 0.26} l${w * 0.2} ${h * 0.16} l${w * 0.14} -${h * 0.1} l${w * 0.24} ${h * 0.2}"/><circle class="${ac ? 'ac' : ''}" cx="${x + w * 0.7}" cy="${y + h * 0.28}" r="${Math.max(5, w * 0.08)}"/></g>`;
	const shelf = (y: number) => `<path d="M-40 ${y}H360"/>`;
	const libLine = shelf(196) + poster(6, 74, 78, 122) + poster(96, 52, 86, 144) + poster(194, 20, 94, 176, true) + `<g transform="rotate(10 300 196)">${poster(292, 66, 80, 130)}</g>`;
	const libTall = libLine + `<g transform="translate(0 -200)">${shelf(196) + poster(20, 56, 90, 140) + poster(122, 86, 74, 110) + `<g transform="rotate(-8 230 196)">${poster(208, 40, 96, 156)}</g>`}</g>`;
	const rc = (x: number, y: number, w: number, h: number, p: number, ac = false) => {
		const pw = ((h - 14) * 2) / 3;
		return `<rect class="o" x="${x}" y="${y}" width="${w}" height="${h}" rx="8"/><rect class="o" x="${x + 8}" y="${y + 7}" width="${pw}" height="${h - 14}" rx="3"/><path d="M${x + pw + 22} ${y + 16}h${w * 0.42}"/><path class="fa" d="M${x + pw + 22} ${y + 28}h${w * 0.26}"/><path class="fa" d="M${x + pw + 22} ${y + h - 14}h${w - pw - 36}"/><path class="${ac ? 'ac run' : ''}" d="M${x + pw + 22} ${y + h - 14}h${(w - pw - 36) * p}"/>`;
	};
	const dlLine = rc(64, 8, 280, 54, 1) + rc(40, 70, 290, 58, 0.7) + rc(16, 136, 304, 64, 0.42, true) + `<path class="ac" d="M28 18v34m-10-10 10 10 10-10"/>`;
	const bust = (x: number, y: number, r: number, ac = false) => `<path class="o${ac ? ' ac' : ''}" d="M${x - r * 2.1} 214C${x - r * 2.1} ${y + r * 1.3} ${x + r * 2.1} ${y + r * 1.3} ${x + r * 2.1} 214Z"/><circle class="o${ac ? ' ac' : ''}" cx="${x}" cy="${y}" r="${r}"/>`;
	const pplLine = bust(66, 96, 26) + bust(254, 90, 26) + bust(160, 106, 34, true) + `<path class="fa dash" d="M-20 206H340"/>`;
</script>

<span class="sx-art" data-g={group} aria-hidden="true">
	{#if group === 'library'}
		{#if shelfP.length}
			<span class="shelf">{#each shelfP as src, i (i)}<img {src} alt="" decoding="async" />{/each}</span>
		{:else}
			<svg class="la w" viewBox="0 0 320 200" preserveAspectRatio="xMidYMax slice">{@html libLine}</svg>
			<svg class="la t" viewBox="40 -150 240 350" preserveAspectRatio="xMidYMax slice">{@html libTall}</svg>
		{/if}
	{:else if group === 'downloads'}
		{#if rel.length}
			<span class="rel">
				{#each rel as src, i (i)}
					<span class="r" style="--p: {['100%', '72%', '38%'][i]}"><img {src} alt="" decoding="async" /><span class="ln"><i></i><i></i></span><span class="pg"><b></b></span></span>
				{/each}
			</span>
		{:else}
			<svg class="la" viewBox="0 0 320 200" preserveAspectRatio="xMidYMax slice">{@html dlLine}</svg>
		{/if}
	{:else if group === 'users'}
		{#if crowd.length}
			<span class="ppl">
				{#each crowd as p, i (p.name + i)}
					<span class="p" style="--bg: {TINTS[i % 4]}; z-index: {10 - i}" class:me={i === 0}>
						<span class="av">{#if p.avatar}<img src={coverFor(p.avatar, 160).src} alt="" decoding="async" />{:else}{initials(p.name)}{/if}</span><span class="nm">{p.name}</span>
					</span>
				{/each}
				{#if more > 0}<span class="p more"><span class="av">+{more}</span></span>{/if}
			</span>
		{:else}
			<svg class="la" viewBox="0 0 320 200" preserveAspectRatio="xMidYMax slice">{@html pplLine}</svg>
		{/if}
	{:else if group === 'connections'}
		<span class="apps">
			<svg class="cab" viewBox="0 0 100 100" preserveAspectRatio="none"><path d="M50 50 C40 38 28 30 20 26M50 50 C60 38 72 30 80 26M50 50 C40 62 28 70 20 74M50 50 C60 62 72 70 80 74" /></svg>
			{#each LOGOS as l (l.k)}
				<span class="tile" style="--x: {l.x}; --y: {l.y}"><svg viewBox="0 0 24 24"><path fill={l.color} d={l.path} /></svg></span>
			{/each}
			<span class="tile hub"><img src={to('/brand/mark-color-on-dark.svg')} alt="" /></span>
		</span>
	{:else if group === 'system'}
		<span class="srv">
			<svg class="pulse" viewBox="0 0 400 60" preserveAspectRatio="none"><path class="base" d="M0 34H120l10-20 12 38 10-30 8 12H260l8-10 8 10H400" /><path d="M0 34H120l10-20 12 38 10-30 8 12H260l8-10 8 10H400" /></svg>
			{#each [0, 1, 2] as k (k)}<span class="u" style="--k: {k}"><span class="led"></span><span class="bay"></span><span class="vent"></span><span class="bay"></span></span>{/each}
		</span>
	{/if}
</span>

<style>
	/* The art box: everything under the words, edge to edge, sized per card. */
	.sx-art {
		--art-top: 88px;
		--tile: color-mix(in srgb, var(--surface-1) 92%, transparent);
		--tile-line: var(--line);
		--art-shadow: rgb(0 0 0 / 0.22);
		--ln: var(--text-muted);
		--occl: var(--surface-1);
		position: absolute; left: 0; right: 0; bottom: 0; top: var(--art-top);
		display: block; overflow: hidden; pointer-events: none; container-type: size;
		/* Hover lifts the whole box (the card sets the scale), never its pieces. */
		transform-origin: 50% 100%;
	}
	.sx-art[data-g='library'] { --art-top: 104px; }
	@media (max-width: 767px) {
		.sx-art { --art-top: 66px; }
		.sx-art[data-g='library'] { --art-top: 70px; }
	}
	.sx-art > :global(*) { position: absolute; inset: 0; }
	.sx-art img { display: block; }
	svg { display: block; width: 100%; height: 100%; }

	/* Library: a poster wall turned a little, running past every edge. */
	.shelf img { position: absolute; display: block; aspect-ratio: 2 / 3; object-fit: cover; border-radius: 10px; box-shadow: 0 10px 28px var(--art-shadow); outline: 1px solid var(--tile-line); outline-offset: -1px; background: var(--surface-2); }

	.shelf { inset: -30cqh -30cqw; display: grid; grid-template-columns: repeat(4, 1fr); gap: 2.4cqw; align-content: center; transform: rotate(-12deg); }
	.shelf img { position: static; width: 100%; }
	.shelf img:nth-child(odd) { transform: translateY(18%); }
	@container (orientation: landscape) { .shelf { grid-template-columns: repeat(6, 1fr); } }

	/* Downloads: three releases coming down, filling the box. */
	.rel .r { position: absolute; overflow: hidden; background: var(--tile); border: 1px solid var(--tile-line); box-shadow: 0 1cqh 4cqh var(--art-shadow); border-radius: 12px; }
	.rel img { display: block; aspect-ratio: 2 / 3; object-fit: cover; background: var(--surface-2); }
	.rel .ln { display: flex; flex-direction: column; gap: 2.6cqh; min-width: 0; }
	.rel .ln i { display: block; height: max(5px, 2.6cqh); border-radius: 2cqh; background: var(--line-strong, var(--line)); }
	.rel .ln i:nth-child(2) { width: 60%; background: var(--line); }
	.rel .pg { position: relative; display: block; height: max(5px, 2.2cqh); border-radius: 2cqh; background: var(--line); overflow: hidden; }
	.rel .pg b { position: absolute; inset: 0 auto 0 0; width: var(--p); background: var(--text-muted); border-radius: inherit; overflow: hidden; }
	.rel .r:nth-child(3) .pg b { background: var(--accent); }
	.rel .r:nth-child(3) .pg b::after { content: ''; position: absolute; inset: 0; background: linear-gradient(90deg, transparent, rgb(255 255 255 / 0.55), transparent); transform: translateX(-100%); animation: sweep 2.4s var(--ease-glide) infinite; }

	.rel .r { left: calc((2 - var(--k)) * 4cqw + 5cqw); right: 5cqw; height: calc((100cqh - 5cqh - 16px) / 3); top: calc(var(--k) * ((100cqh - 5cqh - 16px) / 3 + 8px)); display: grid; grid-template-columns: auto 1fr; grid-template-rows: 1fr auto; column-gap: 4cqw; padding-right: 4cqw; }
	.rel img { height: 100%; grid-row: span 2; }
	.rel .ln { align-self: end; padding-bottom: 1.6cqh; }
	.rel .pg { margin-bottom: 18%; }
	.rel .r:nth-child(1) { --k: 0; }
	.rel .r:nth-child(2) { --k: 1; }
	.rel .r:nth-child(3) { --k: 2; }
	.rel .r:nth-child(1) { opacity: 0.7; }
	.rel .r:nth-child(2) { opacity: 0.88; }

	.rel .r:nth-child(3)::after { content: none; }

	/* People: you in the middle, the others around you. */
	.ppl .p { position: absolute; display: flex; flex-direction: column; align-items: center; gap: 1.6cqh; min-width: 0; }
	.ppl .av { display: grid; place-items: center; width: var(--s); aspect-ratio: 1; border-radius: 50%; overflow: hidden; background: var(--bg, var(--surface-2)); color: #fff; font: 600 calc(var(--s) * 0.36) / 1 var(--font-display, var(--font-ui)); border: 3px solid var(--occl); box-shadow: 0 4px 14px var(--art-shadow); }
	.ppl .av img { width: 100%; height: 100%; object-fit: cover; }
	.ppl .me .av { box-shadow: 0 0 0 2px var(--accent), 0 4px 14px var(--art-shadow); }
	.ppl .more .av { background: var(--surface-2); color: var(--text); font-size: calc(var(--s) * 0.3); }
	.ppl .nm { max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); font: 500 12px/1.2 var(--font-ui); }

	.ppl { --s: min(54cqh, 38cqw); --o: min(32cqh, 26cqw); }
	.ppl .p { left: 50%; top: 47%; transform: translate(-50%, -50%); }
	.ppl .p:not(.me) { --s: min(26cqh, 18cqw); }
	.ppl .p:nth-child(2) { left: calc(50% - var(--o) * 1.25); top: calc(47% - var(--o) * 0.6); }
	.ppl .p:nth-child(3) { left: calc(50% + var(--o) * 1.25); top: calc(47% - var(--o) * 0.5); }
	.ppl .p:nth-child(4) { left: calc(50% - var(--o) * 1.15); top: calc(47% + var(--o) * 0.75); }
	.ppl .p:nth-child(5), .ppl .more { left: calc(50% + var(--o) * 1.2); top: calc(47% + var(--o) * 0.7); }
	.ppl .p:nth-child(5) ~ .more { display: none; }
	.ppl .nm { display: none; }

	@keyframes sweep { to { transform: translateX(100%); } }

	/* Linked apps: the apps it talks to, wired to HolyArrs. */
	.apps .cab path { fill: none; stroke: var(--line-strong, var(--line)); stroke-width: 1.5; vector-effect: non-scaling-stroke; stroke-dasharray: 4 5; }
	.apps .tile { position: absolute; width: min(30cqh, 21cqw); aspect-ratio: 1; border-radius: 24%; background: var(--tile); border: 1px solid var(--tile-line); box-shadow: 0 1.2cqh 4cqh var(--art-shadow); display: grid; place-items: center; left: calc(var(--x) - min(15cqh, 10.5cqw)); top: calc(var(--y) - min(15cqh, 10.5cqw)); }
	.apps .tile svg { width: 56%; height: 56%; }
	.apps .hub { width: min(38cqh, 26cqw); left: calc(50% - min(19cqh, 13cqw)); top: calc(50% - min(19cqh, 13cqw)); z-index: 2; background: #1c1d21; border-color: rgb(255 255 255 / 0.12); }
	.apps .hub img { width: 62%; height: 62%; }

	/* Server: a rack with a slow heartbeat. */
	.srv .u { position: absolute; left: 8cqw; right: 8cqw; height: 18cqh; top: calc(var(--k) * 23cqh + 30cqh); background: linear-gradient(180deg, var(--tile), color-mix(in srgb, var(--tile) 80%, var(--text) 8%)); border: 1px solid var(--tile-line); border-radius: 3cqh; box-shadow: 0 1cqh 4cqh var(--art-shadow); display: flex; align-items: center; gap: 3cqw; padding: 0 4cqw; }
	.srv .vent { flex: 1; height: 44%; background: repeating-linear-gradient(90deg, var(--line-strong, var(--line)) 0 2px, transparent 2px 7px); border-radius: 2px; opacity: 0.8; }
	.srv .bay { width: 14cqw; height: 44%; border-radius: 1.5cqh; background: var(--line); border: 1px solid var(--tile-line); }
	.srv .led { width: max(5px, 2.4cqh); aspect-ratio: 1; border-radius: 50%; background: var(--text-muted); }
	.srv .u:first-of-type .led { background: var(--accent); box-shadow: 0 0 2.5cqh var(--accent); }
	.srv .pulse { inset: 0 0 auto 0; height: 24cqh; }
	.srv .pulse path { fill: none; stroke: var(--accent); stroke-width: 2; vector-effect: non-scaling-stroke; stroke-linecap: round; stroke-linejoin: round; stroke-dasharray: 60 340; animation: pulse 3.6s linear infinite; }
	.srv .pulse .base { stroke: var(--line-strong, var(--line)); stroke-dasharray: none; animation: none; stroke-width: 1.2; }
	@keyframes pulse { from { stroke-dashoffset: 400; } to { stroke-dashoffset: 0; } }

	/* Line art fallback: one weight, one red. */
	.la { fill: none; stroke: var(--ln); stroke-width: 1.6; stroke-linecap: round; stroke-linejoin: round; }
	.la :global(*) { vector-effect: non-scaling-stroke; }
	.la :global(.o) { fill: var(--occl); }
	.la :global(.ac) { stroke: var(--accent); }
	.la :global(.fa) { stroke: var(--line-strong, var(--line)); }
	.la :global(.dash) { stroke-dasharray: 3 5; }
	.la :global(.run) { stroke-dasharray: 46 300; animation: pulse 4s linear infinite; }
	.la.t { display: none; }
	@container (orientation: portrait) { .la.w { display: none; } .la.t { display: block; } }

	@media (prefers-reduced-motion: reduce) {
		.rel .r:nth-child(3) .pg b::after, .srv .pulse path, .la :global(.run) { animation: none; }
	}
</style>
