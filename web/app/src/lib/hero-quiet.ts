// The Explore hero's words, as Netflix runs them: a few seconds into the
// trailer the overview folds away and the logo glides down; the words come
// back when the trailer stops for good (ended, failed, or taken over by
// another player) or the title changes. Pointer moves and scrolling do not
// change it. Every state can go to every other, so nothing gets stuck.

export type HeroPhase = 'still' | 'playing' | 'quiet' | 'ended';
export type HeroEvent = 'play' | 'settle' | 'stop' | 'reset';

/** Seconds of trailer before the words fold away (measured: about 6 s). */
export const QUIET_AFTER = 6000;

export function heroStep(phase: HeroPhase, ev: HeroEvent): HeroPhase {
	switch (ev) {
		case 'reset':
			return 'still';
		case 'stop':
			return phase === 'still' ? 'still' : 'ended';
		case 'play':
			// Resuming after a pause keeps the words folded; a fresh start shows them.
			return phase === 'quiet' ? 'quiet' : 'playing';
		case 'settle':
			return phase === 'playing' ? 'quiet' : phase;
	}
}

/** The overview and facts are shown in every phase but quiet. */
export const wordsShown = (p: HeroPhase) => p !== 'quiet';

/** A tiny driver: the phase plus the settle timer, with injectable timers for tests. */
export function heroQuiet(
	onchange: (p: HeroPhase) => void,
	timers: { set: (f: () => void, ms: number) => ReturnType<typeof setTimeout>; clear: (t: ReturnType<typeof setTimeout>) => void } = {
		set: (f, ms) => setTimeout(f, ms),
		clear: (t) => clearTimeout(t)
	}
) {
	let phase: HeroPhase = 'still';
	let t: ReturnType<typeof setTimeout> | undefined;
	const go = (ev: HeroEvent) => {
		const next = heroStep(phase, ev);
		if (t !== undefined && next !== 'playing') {
			timers.clear(t);
			t = undefined;
		}
		if (next === 'playing' && phase !== 'playing') t = timers.set(() => ((t = undefined), go('settle')), QUIET_AFTER);
		if (next !== phase) {
			phase = next;
			onchange(phase);
		}
	};
	return { send: go, get phase() { return phase; }, dispose: () => t !== undefined && timers.clear(t) };
}
