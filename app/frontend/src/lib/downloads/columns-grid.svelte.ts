import { tick } from 'svelte';
import { COL, COLUMNS, IDS, KEY, MAX_W, MIN_W, OLD_KEY, defaults, dump, load, move, moveVisible, nextSort, toggle as toggleCol, visible, type ColId, type Layout, type SwarmFacet } from './columns.ts';
import type { DownloadRow } from '../core/types.ts';

/** What the column grid needs from the page that owns it. */
export type ColumnsContext = {
	isDone: () => boolean;
	list: () => DownloadRow[];
};

/** The column layout, the list's sizing and scrolling window, and the column picker. */
export function createColumns(ctx: ColumnsContext) {
	const isDone = $derived(ctx.isDone());
	const list = $derived(ctx.list());
	// Columns: which ones show, their order and their widths are each person's
	// own, kept in this browser (columns.ts). Every row of both sections shares
	// one grid: ▾ · poster · the shown columns in order · spare room · bin.
	// Dragging a column's right edge changes that column only; dragging its name
	// moves it; right-click a name, or ⋯ → Columns, opens the picker.
	let grid = $state<HTMLElement>();
	let appEl = $state<HTMLElement>();
	let listEl = $state<HTMLElement>();
	let appH = $state(0);
	let phone = $state(false);
	// A long list (over 100 rows) draws only the rows on screen plus 10 on each
	// side; the list keeps its full height with padding above and below, so
	// the scroll bar, sticky header and column widths stay as they are.
	const WINDOW_FROM = 100;
	const OVERSCAN = 10;
	let ulEl = $state<HTMLElement>();
	let vTop = $state(0);
	let vH = $state(900);
	let ulTop = $state(0);
	let rowH = $state(0);
	const windowed = $derived(list.length > WINDOW_FROM);
	const rh = $derived(rowH || (phone ? 65 : 45));
	const span = $derived.by(() => {
		const n = list.length;
		if (!windowed) return { a: 0, b: n };
		const off = vTop - ulTop;
		const b = Math.min(n, Math.max(0, Math.ceil((off + vH) / rh)) + OVERSCAN);
		let a = Math.min(Math.max(0, Math.floor(off / rh) - OVERSCAN), Math.max(0, b - 1));
		// Start on an even row so the striped rows keep their colours.
		a -= a % 2;
		return { a, b };
	});
	const shownRows = $derived(windowed ? list.slice(span.a, span.b) : list);
	function measure() {
		if (!listEl) return;
		vTop = listEl.scrollTop;
		vH = listEl.clientHeight || vH;
		if (ulEl) ulTop = ulEl.getBoundingClientRect().top - listEl.getBoundingClientRect().top + listEl.scrollTop;
	}
	$effect(() => {
		const el = listEl;
		if (!el || !windowed) return;
		let raf = 0;
		const onScroll = () => {
			if (!raf) raf = requestAnimationFrame(() => ((raf = 0), measure()));
		};
		measure();
		el.addEventListener('scroll', onScroll, { passive: true });
		const ro = new ResizeObserver(measure);
		ro.observe(el);
		return () => {
			cancelAnimationFrame(raf);
			el.removeEventListener('scroll', onScroll);
			ro.disconnect();
		};
	});
	// The details below the list fit their content, so the list can get shorter
	// after a row was scrolled into view (its details arrive): keep the focused row on screen.
	$effect(() => {
		const el = listEl;
		if (!el || typeof ResizeObserver === 'undefined') return;
		let h = el.clientHeight;
		const ro = new ResizeObserver(() => {
			const was = h;
			h = el.clientHeight;
			const a = document.activeElement as HTMLElement | null;
			if (h < was && a?.classList.contains('dline') && el.contains(a)) a.scrollIntoView({ block: 'nearest' });
		});
		ro.observe(el);
		return () => ro.disconnect();
	});
	// The real row height (desktop and phone rows differ), read from a drawn row.
	$effect(() => {
		void shownRows;
		void phone;
		if (!windowed || !ulEl) return;
		tick().then(() => {
			const h = ulEl?.querySelector<HTMLElement>('.drow')?.offsetHeight;
			if (h && h !== rowH) rowH = h;
		});
	});
	/** reveal scrolls a row that is not drawn into the window, so it can be focused. */
	function reveal(i: number) {
		if (!windowed || !listEl || (i >= span.a && i < span.b)) return;
		measure();
		listEl.scrollTop = Math.max(0, ulTop + i * rh - vH / 2);
		vTop = listEl.scrollTop;
	}
	let gap = $state(10);
	let layout = $state<Layout>(typeof localStorage === 'undefined' ? defaults() : readLayout());
	function readLayout(): Layout {
		try {
			return load(localStorage.getItem(KEY), localStorage.getItem(OLD_KEY));
		} catch {
			return defaults();
		}
	}
	function save() {
		try {
			localStorage.setItem(KEY, dump(layout));
		} catch {
			/* private window: the layout lasts for this visit */
		}
	}
	// A layout nobody has changed shows more columns when the screen is wide (they drop out again when it is not).
	const customised = $derived(layout.hidden.length !== COLUMNS.filter((c) => !c.on).length || COLUMNS.some((c) => !c.on && !layout.hidden.includes(c.id)) || layout.order.some((id, i) => id !== IDS[i]));
	// One layout everywhere: wide screens no longer add columns by themselves.
	const WIDE: ColId[] = [];
	// Done has its own default columns: finished rows have no speed, but a date.
	const DONE_SET: ColId[] = ['poster', 'title', 'proto', 'status', 'size', 'added', 'requested'];
	const visAll = $derived.by(() => {
		if (isDone && !customised) return DONE_SET.filter((id) => !COL[id].missing);
		const v = visible(layout);
		return customised ? v : [...v, ...WIDE.filter((id) => !v.includes(id) && !COL[id].missing)];
	});
	/** The least width each number column needs to show its text whole. */
	const MINW: Partial<Record<ColId, number>> = { poster: 32, kind: 24, proto: 24, progress: 130, size: 56, swarm: 132, added: 70, status: 100 };
	const TITLE_MIN = 220;
	// A saved width never draws narrower than the column's own name (older layouts, or a sort arrow added later).
	const wMin = (id: ColId) => (id === 'poster' ? posterW : id === 'kind' ? kindW : id === 'proto' ? srcW : COL[id].fixed ? COL[id].width : Math.max(layout.widths[id], heads[id] ?? 0));
	/** heads holds each header's name width, measured after layout and after each sort. */
	let heads = $state<Partial<Record<ColId, number>>>({});
	// The Source column hugs the client's logo; its header is an icon (wider only while a sort arrow shows).
	const SRC_W = 24;
	/** kindIcon turns a type word (matched, or read from the client's category) into its icon; anything else has none. */
	const kindIcon = (w: string) => (w === 'Movie' ? 'film' : w === 'TV show' ? 'tv' : /music|audio|lidarr/i.test(w) ? 'music' : null);
	// Posters come in three sizes (small, medium, large); dragging the Poster edge snaps between them and the rows grow to fit.
	const POSTER_W = [32, 46, 62];
	// Type has two sizes: an icon, or (dragged wider) a coloured word.
	const KIND_W = [24, 60];
	// Source has three sizes: small logo, bigger logo, and logo with the client's name.
	const SRC_SIZES = [24, 30, 92];
	const srcW = $derived(SRC_SIZES.includes(layout.widths.proto) ? layout.widths.proto : SRC_SIZES[0]);
	const srcBig = $derived(srcW > SRC_SIZES[0]);
	const srcName = $derived(srcW === SRC_SIZES[2]);
	const kindW = $derived(KIND_W.includes(layout.widths.kind) ? layout.widths.kind : KIND_W[0]);
	const kindWide = $derived(kindW > KIND_W[0]);
	const posterW = $derived(POSTER_W.includes(layout.widths.poster) ? layout.widths.poster : POSTER_W[0]);
	const near = (set: number[], w: number) => set.reduce((a, b) => (Math.abs(b - w) < Math.abs(a - w) ? b : a));
	// Every picked column shows; when they are wider than the view the list scrolls sideways.
	const vis = $derived(visAll);
	const FACETS: { id: SwarmFacet; label: string }[] = [{ id: 'seeds', label: 'Seeds' }, { id: 'peers', label: 'Peers' }, { id: 'ratio', label: 'Ratio' }];
	const ordinal = (n: number) => { const r = n % 100; return `${n}${r >= 11 && r <= 13 ? 'th' : (['th', 'st', 'nd', 'rd'][n % 10] ?? 'th')}`; };
	// A heading's accessible name: its column and how the list is sorted by it.
	function headName(id: ColId, so: { desc: boolean } | undefined): string {
		const label = id === 'proto' ? 'Downloader' : id === 'poster' ? 'Poster' : id === 'kind' ? 'Type' : id === 'swarm' ? 'Seeds, peers and ratio' : COL[id].label;
		return so ? `${label}, sorted ${so.desc ? 'high to low' : 'low to high'}` : label;
	}
	// Column headings take one Tab stop: Left and Right move between them, Enter or Space sorts.
	// A screen reader's click (no pointer, detail 0) sorts too.
	function headKey(e: KeyboardEvent, id: ColId) {
		if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			sortBy(id);
			return;
		}
		if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
		const heads = [...(e.currentTarget as HTMLElement).parentElement!.querySelectorAll<HTMLElement>('[data-testid="dl-head"]')];
		const next = heads[heads.indexOf(e.currentTarget as HTMLElement) + (e.key === 'ArrowRight' ? 1 : -1)];
		if (!next) return;
		e.preventDefault();
		heads.forEach((h) => (h.tabIndex = h === next ? 0 : -1));
		next.focus();
	}
	function sortBy(id: ColId, facet?: SwarmFacet) {
		layout.sort = nextSort(layout.sort, id, facet);
		save();
	}
	/** A phone shows a fixed card (title, bar, status, time) whatever is picked. */
	const PHONE: ColId[] = ['poster', 'title', 'progress', 'status', 'size'];
	const cells = $derived([...vis.map((id) => ({ id, ph: false })), ...PHONE.filter((id) => !vis.includes(id)).map((id) => ({ id, ph: true }))]);
	const FIXED = 20 + 32;
	let room = $state(0);
	let titleMin = $state(MIN_W);
	/** wOf is a column's width on screen (an unset Title gives way when room is short). */
	const tW = $derived.by(() => {
		if (!vis.includes('title')) return 0;
		if (room <= 0 || layout.titleSet) return wMin('title');
		// A Title nobody has sized takes what is left (with a little slack); a sized one only gives way when the room is short.
		const rest = vis.reduce((n, id) => (id === 'title' ? n : n + wMin(id)), (layout.titleSet ? 0 : 48) + FIXED + (vis.length + 3) * gap);
		return Math.max(TITLE_MIN, Math.min(layout.widths.title, Math.floor(room - rest)));
	});
	const wOf = (id: ColId) => (id === 'title' ? tW : wMin(id));
	// Until a person sets the Title width, it flexes and takes whatever room is left.
	const tpl = $derived(`20px ${vis.map((id) => (id === 'title' && !layout.titleSet ? `minmax(${wOf(id)}px, 1fr)` : `${wOf(id)}px`)).join(' ')} ${layout.titleSet || !vis.includes('title') ? 'minmax(0, 1fr)' : '0px'} 32px`);
	/** inner is the full width of the columns; the table box is at least this wide. */
	const inner = $derived(vis.reduce((n, id) => n + wOf(id), FIXED) + (vis.length + 3) * gap);
	const textW = (n: Node) => {
		const r = document.createRange();
		r.selectNodeContents(n);
		return r.getBoundingClientRect().width;
	};
	/** minOf is a column's header label plus padding: any column shrinks to its own name. */
	function minOf(id: ColId): number {
		const l = grid?.querySelector<HTMLElement>(`.chead [data-col="${id}"] .lbl`);
		const w = l ? textW(l) : 0;
		return w > 0 ? Math.ceil(w) + 2 : MIN_W;
	}
	// Dragging stops at the column's own name; MINW only sets how narrow double-click fit goes.
	const clamp = (id: ColId, v: number) => Math.round(Math.max(minOf(id), Math.min(MAX_W, v)));
	/** fitOf is the widest cell of a column (the header label too) plus padding. */
	function fitOf(id: ColId): number {
		if (!grid) return COL[id].width;
		if (id === 'progress') return clamp(id, COL.progress.width); // a bar has no content width of its own
		let w = 0;
		for (const el of grid.querySelectorAll<HTMLElement>(`.dline [data-col="${id}"]`)) {
			if (id === 'title' && !el.classList.contains('ch')) {
				for (const t of el.querySelectorAll('.tt, .rel')) w = Math.max(w, textW(t));
				continue;
			}
			const tx = el.classList.contains('ch') ? el.querySelector('.lbl') : (el.querySelector('.tx, .kt') ?? el);
			w = Math.max(w, tx ? textW(tx) + (el.classList.contains('st') ? 14 : 0) : 0);
		}
		return clamp(id, Math.max(Math.ceil(w) + 14, MINW[id] ?? 0));
	}
	function measureHeads() {
		const m: Partial<Record<ColId, number>> = {};
		for (const id of vis) if (grid?.querySelector(`.chead [data-col="${id}"] .lbl`)) m[id] = minOf(id);
		heads = m;
	}
	// A new sort arrow or column set changes the header widths, so measure again once it has drawn.
	$effect(() => {
		void layout.sort;
		void vis;
		const t = setTimeout(measureHeads, 0);
		return () => clearTimeout(t);
	});
	$effect(() => {
		if (!grid) return;
		gap = parseFloat(getComputedStyle(grid).getPropertyValue('--gap')) || 10;
		const g = grid;
		const ro = new ResizeObserver(() => {
			room = (listEl ?? g).clientWidth;
			phone = matchMedia('(max-width: 640px)').matches;
			titleMin = minOf('title');
			measureHeads();
		});
		ro.observe(g);
		if (listEl) ro.observe(listEl);
		return () => ro.disconnect();
	});
	// The app fills the column down to the bottom of the screen; the list scrolls inside it.
	$effect(() => {
		if (!appEl) return;
		const fit = () => {
			const small = innerWidth <= 767;
			const tab = small ? (parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--tabbar')) || 56) : 0;
			appH = Math.max(420, Math.floor(innerHeight - appEl!.getBoundingClientRect().top - tab));
		};
		fit();
		const ro = new ResizeObserver(fit);
		ro.observe(document.body);
		window.addEventListener('resize', fit);
		return () => {
			ro.disconnect();
			window.removeEventListener('resize', fit);
		};
	});
	// Every table box scrolls sideways together, so the columns stay lined up.
	function sync(e: Event) {
		const x = (e.currentTarget as HTMLElement).scrollLeft;
		for (const t of grid?.querySelectorAll<HTMLElement>('.tw') ?? []) if (t.scrollLeft !== x) t.scrollLeft = x;
	}
	function resize(id: ColId, w0: number, d: number) {
		if (id === 'title') layout.titleSet = true;
		layout.widths[id] = id === 'poster' ? near(POSTER_W, w0 + d) : id === 'kind' ? near(KIND_W, w0 + d) : id === 'proto' ? near(SRC_SIZES, w0 + d) : clamp(id, w0 + d);
	}
	function grab(e: PointerEvent, id: ColId) {
		if (e.button !== 0) return;
		e.preventDefault();
		e.stopPropagation();
		const x0 = e.clientX;
		const el = e.currentTarget as HTMLElement;
		// Start from the width on screen (a flexing Title is wider than its saved width), so the edge stays under the pointer.
		const w0 = Math.round(el.parentElement?.getBoundingClientRect().width ?? wOf(id));
		el.setPointerCapture(e.pointerId);
		el.classList.add('drag');
		document.documentElement.classList.add('col-resizing');
		const mv = (m: PointerEvent) => resize(id, w0, m.clientX - x0);
		const up = () => {
			el.removeEventListener('pointermove', mv);
			el.removeEventListener('pointerup', up);
			el.removeEventListener('pointercancel', up);
			el.classList.remove('drag');
			document.documentElement.classList.remove('col-resizing');
			save();
		};
		el.addEventListener('pointermove', mv);
		el.addEventListener('pointerup', up);
		el.addEventListener('pointercancel', up);
	}
	function nudge(e: KeyboardEvent, id: ColId) {
		const d = e.key === 'ArrowRight' ? 1 : e.key === 'ArrowLeft' ? -1 : 0;
		if (!d) return;
		e.preventDefault();
		resize(id, Math.round((e.currentTarget as HTMLElement).parentElement?.getBoundingClientRect().width ?? wOf(id)), d * (e.shiftKey ? 40 : 10));
		save();
	}
	// Double-click auto-fits that column, as in a spreadsheet.
	function fitOne(id: ColId) {
		if (id === 'title') layout.titleSet = true;
		// Double-click on Poster steps to the next size (large goes back to small); on Type it flips icon and word.
		layout.widths[id] = id === 'poster' ? POSTER_W[(POSTER_W.indexOf(posterW) + 1) % POSTER_W.length] : id === 'kind' ? (kindWide ? KIND_W[0] : KIND_W[1]) : id === 'proto' ? SRC_SIZES[(SRC_SIZES.indexOf(srcW) + 1) % SRC_SIZES.length] : fitOf(id);
		save();
	}
	function resetAll() {
		try {
			localStorage.removeItem(KEY);
			localStorage.removeItem(OLD_KEY);
		} catch {
			/* nothing saved */
		}
		layout = defaults();
	}

	// Dragging a column's name left or right moves it: a ghost follows the
	// pointer and a line shows where it will land.
	let ghost = $state<{ id: ColId; x: number; y: number } | null>(null);
	let lineX = $state<number | null>(null);
	let slot = 0;
	function slotAt(head: HTMLElement, x: number): { slot: number; line: number } {
		const box = head.getBoundingClientRect();
		const hs = [...head.querySelectorAll<HTMLElement>('.ch')].map((h) => h.getBoundingClientRect());
		let i = hs.findIndex((r) => x < r.left + r.width / 2);
		if (i < 0) i = hs.length;
		const at = i < hs.length ? hs[i].left - gap / 2 : hs[hs.length - 1].right + gap / 2;
		return { slot: i, line: at - box.left };
	}
	function headDown(e: PointerEvent, id: ColId) {
		if (e.button !== 0 || (e.target as Element).closest('.grip')) return;
		const el = e.currentTarget as HTMLElement;
		const head = el.closest<HTMLElement>('.chead');
		if (!head) return;
		const x0 = e.clientX;
		let on = false;
		el.setPointerCapture(e.pointerId);
		const mv = (m: PointerEvent) => {
			if (!on && Math.abs(m.clientX - x0) < 5) return;
			on = true;
			ghost = { id, x: m.clientX, y: m.clientY };
			const s = slotAt(head, m.clientX);
			slot = s.slot;
			lineX = s.line;
		};
		const up = (u?: PointerEvent) => {
			if (!on && u?.type === 'pointerup') sortBy(id);
			el.removeEventListener('pointermove', mv);
			el.removeEventListener('pointerup', up);
			el.removeEventListener('pointercancel', stop);
			if (on) {
				const from = vis.indexOf(id);
				const to = slot > from ? slot - 1 : slot;
				if (to !== from) {
					layout.order = moveVisible(layout, id, to);
					save();
				}
			}
			ghost = null;
			lineX = null;
		};
		const stop = () => {
			on = false;
			up();
		};
		el.addEventListener('pointermove', mv);
		el.addEventListener('pointerup', up);
		el.addEventListener('pointercancel', stop);
	}

	// The column picker: every column with a tick and a handle to reorder.
	let pop = $state<{ x: number; y: number } | null>(null);
	let pickEl = $state<HTMLElement>();
	let held = $state<ColId | null>(null);
	let heldFrom: ColId[] = [];
	let said = $state('');
	function openPicker(x: number, y: number) {
		const w = 260;
		pop = { x: Math.max(8, Math.min(x, innerWidth - w - 8)), y: Math.max(8, Math.min(y, innerHeight - 440)) };
		held = null;
		tick().then(() => {
			// Keep the whole list on screen: move it up by however much its
			// bottom passes the screen edge (it scrolls inside past that).
			const r = pickEl?.getBoundingClientRect();
			if (r && pop && r.bottom > innerHeight - 8) pop = { x: pop.x, y: Math.max(8, pop.y - (r.bottom - (innerHeight - 8))) };
			pickEl?.querySelector<HTMLElement>('input, button')?.focus();
		});
	}
	function closePicker() {
		if (held) layout.order = heldFrom;
		held = null;
		pop = null;
		save();
	}
	function headMenu(e: MouseEvent) {
		e.preventDefault();
		openPicker(e.clientX, e.clientY);
	}
	function fromMenu() {
		const b = grid?.querySelector<HTMLElement>('.cm button');
		const r = b?.getBoundingClientRect();
		openPicker(r ? r.right - 260 : innerWidth / 2, r ? r.bottom + 6 : 80);
	}
	function outside(e: PointerEvent) {
		if (pop && pickEl && !pickEl.contains(e.target as Node)) closePicker();
	}
	function flip(id: ColId) {
		layout.hidden = toggleCol(layout, id);
		save();
	}
	async function refocus(id: ColId) {
		await tick();
		pickEl?.querySelector<HTMLElement>(`[data-handle="${id}"]`)?.focus();
	}
	function handleKey(e: KeyboardEvent, id: ColId) {
		const i = layout.order.indexOf(id);
		if (e.key === ' ' || e.key === 'Enter') {
			e.preventDefault();
			if (held === id) {
				held = null;
				said = `${COL[id].label} dropped at position ${i + 1}.`;
				save();
			} else if (e.key === ' ') {
				heldFrom = [...layout.order];
				held = id;
				said = `${COL[id].label} picked up. Use the arrow keys to move it, Enter to drop.`;
			}
		} else if (held === id && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) {
			e.preventDefault();
			const to = i + (e.key === 'ArrowUp' ? -1 : 1);
			if (to < 0 || to >= layout.order.length) return;
			layout.order = move(layout.order, id, to);
			said = `${COL[id].label}: position ${to + 1} of ${layout.order.length}.`;
			refocus(id);
		} else if (e.key === 'Escape' && held) {
			e.preventDefault();
			e.stopPropagation();
			layout.order = heldFrom;
			held = null;
			said = 'Move cancelled.';
			refocus(id);
		}
	}
	function handleDown(e: PointerEvent, id: ColId) {
		if (e.button !== 0) return;
		e.preventDefault();
		const el = e.currentTarget as HTMLElement;
		el.setPointerCapture(e.pointerId);
		held = id;
		const mv = (m: PointerEvent) => {
			const lis = [...(pickEl?.querySelectorAll<HTMLElement>('li[data-id]') ?? [])];
			let to = lis.findIndex((li) => m.clientY < li.getBoundingClientRect().top + li.offsetHeight / 2);
			if (to < 0) to = lis.length - 1;
			const from = layout.order.indexOf(id);
			if (to > from) to -= 1;
			if (to !== from) layout.order = move(layout.order, id, to);
		};
		const up = () => {
			el.removeEventListener('pointermove', mv);
			el.removeEventListener('pointerup', up);
			el.removeEventListener('pointercancel', up);
			held = null;
			save();
		};
		el.addEventListener('pointermove', mv);
		el.addEventListener('pointerup', up);
		el.addEventListener('pointercancel', up);
	}
	function pickerKey(e: KeyboardEvent) {
		if (e.key === 'Escape' && !held) {
			e.preventDefault();
			closePicker();
			grid?.querySelector<HTMLElement>('.cm button')?.focus();
		}
	}

	return {
		get grid() { return grid; },
		set grid(v) { grid = v; },
		get appEl() { return appEl; },
		set appEl(v) { appEl = v; },
		get listEl() { return listEl; },
		set listEl(v) { listEl = v; },
		get appH() { return appH; },
		set appH(v) { appH = v; },
		get phone() { return phone; },
		set phone(v) { phone = v; },
		get WINDOW_FROM() { return WINDOW_FROM; },
		get OVERSCAN() { return OVERSCAN; },
		get ulEl() { return ulEl; },
		set ulEl(v) { ulEl = v; },
		get vTop() { return vTop; },
		set vTop(v) { vTop = v; },
		get vH() { return vH; },
		set vH(v) { vH = v; },
		get ulTop() { return ulTop; },
		set ulTop(v) { ulTop = v; },
		get rowH() { return rowH; },
		set rowH(v) { rowH = v; },
		get windowed() { return windowed; },
		get rh() { return rh; },
		get span() { return span; },
		get shownRows() { return shownRows; },
		get measure() { return measure; },
		get reveal() { return reveal; },
		get gap() { return gap; },
		set gap(v) { gap = v; },
		get layout() { return layout; },
		set layout(v) { layout = v; },
		get readLayout() { return readLayout; },
		get save() { return save; },
		get customised() { return customised; },
		get WIDE() { return WIDE; },
		get DONE_SET() { return DONE_SET; },
		get visAll() { return visAll; },
		get MINW() { return MINW; },
		get TITLE_MIN() { return TITLE_MIN; },
		get wMin() { return wMin; },
		get heads() { return heads; },
		set heads(v) { heads = v; },
		get SRC_W() { return SRC_W; },
		get kindIcon() { return kindIcon; },
		get POSTER_W() { return POSTER_W; },
		get KIND_W() { return KIND_W; },
		get SRC_SIZES() { return SRC_SIZES; },
		get srcW() { return srcW; },
		get srcBig() { return srcBig; },
		get srcName() { return srcName; },
		get kindW() { return kindW; },
		get kindWide() { return kindWide; },
		get posterW() { return posterW; },
		get near() { return near; },
		get vis() { return vis; },
		get FACETS() { return FACETS; },
		get ordinal() { return ordinal; },
		get headName() { return headName; },
		get headKey() { return headKey; },
		get sortBy() { return sortBy; },
		get PHONE() { return PHONE; },
		get cells() { return cells; },
		get FIXED() { return FIXED; },
		get room() { return room; },
		set room(v) { room = v; },
		get titleMin() { return titleMin; },
		set titleMin(v) { titleMin = v; },
		get tW() { return tW; },
		get wOf() { return wOf; },
		get tpl() { return tpl; },
		get inner() { return inner; },
		get textW() { return textW; },
		get minOf() { return minOf; },
		get clamp() { return clamp; },
		get fitOf() { return fitOf; },
		get measureHeads() { return measureHeads; },
		get sync() { return sync; },
		get resize() { return resize; },
		get grab() { return grab; },
		get nudge() { return nudge; },
		get fitOne() { return fitOne; },
		get resetAll() { return resetAll; },
		get ghost() { return ghost; },
		set ghost(v) { ghost = v; },
		get lineX() { return lineX; },
		set lineX(v) { lineX = v; },
		get slot() { return slot; },
		set slot(v) { slot = v; },
		get slotAt() { return slotAt; },
		get headDown() { return headDown; },
		get pop() { return pop; },
		set pop(v) { pop = v; },
		get pickEl() { return pickEl; },
		set pickEl(v) { pickEl = v; },
		get held() { return held; },
		set held(v) { held = v; },
		get heldFrom() { return heldFrom; },
		set heldFrom(v) { heldFrom = v; },
		get said() { return said; },
		set said(v) { said = v; },
		get openPicker() { return openPicker; },
		get closePicker() { return closePicker; },
		get headMenu() { return headMenu; },
		get fromMenu() { return fromMenu; },
		get outside() { return outside; },
		get flip() { return flip; },
		get refocus() { return refocus; },
		get handleKey() { return handleKey; },
		get handleDown() { return handleDown; },
		get pickerKey() { return pickerKey; }
	};
}

/** The column grid's state, as the parts that draw it receive it. */
export type Columns = ReturnType<typeof createColumns>;
