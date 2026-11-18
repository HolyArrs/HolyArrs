// A stand-in window: frames run when the test says, and input events can be sent.
export function fakeWindow() {
	const frames = new Map<number, () => void>();
	const on = new Map<string, Set<(e: Event) => void>>();
	let n = 0;
	return {
		scrollY: 0,
		innerHeight: 900,
		innerWidth: 1280,
		requestAnimationFrame: (f: () => void) => (frames.set(++n, f), n),
		cancelAnimationFrame: (id: number) => void frames.delete(id),
		addEventListener: (e: string, f: (e: Event) => void) => void (on.get(e) ?? on.set(e, new Set()).get(e)!).add(f),
		removeEventListener: (e: string, f: (e: Event) => void) => void on.get(e)?.delete(f),
		frame() {
			const run = [...frames.values()];
			frames.clear();
			run.forEach((f) => f());
		},
		// A real event; a key press carries its key and whether it is a held key's repeat (the test runs without a DOM, so a plain Event stands in for a KeyboardEvent).
		send: (e: string, init: { key?: string; repeat?: boolean; altKey?: boolean; metaKey?: boolean; button?: number } = {}) => {
			const ev = Object.assign(new Event(e), { key: '', repeat: false }, init);
			[...(on.get(e) ?? [])].forEach((f) => f(ev));
		},
		listening: () => [...on.values()].reduce((a, s) => a + s.size, 0),
		pending: () => frames.size
	};
}
