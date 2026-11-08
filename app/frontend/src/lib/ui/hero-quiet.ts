// Hero trailer phases: words hide once the video has played a while.


export type HeroPhase = 'still' | 'playing' | 'quiet' | 'ended';
export type HeroEvent = 'play' | 'settle' | 'stop' | 'reset';

// ms of playing before the words fade.
export const QUIET_AFTER = 6000;

// Quiet stays quiet until reset; stop never leaves still or quiet.
export function heroStep(phase: HeroPhase, ev: HeroEvent): HeroPhase {
	switch (ev) {
		case 'reset':
			return 'still';
		case 'stop':
			return phase === 'still' || phase === 'quiet' ? phase : 'ended';
		case 'play':
			return phase === 'quiet' ? 'quiet' : 'playing';
		case 'settle':
			return phase === 'playing' ? 'quiet' : phase;
	}
}

export const wordsShown = (p: HeroPhase) => p !== 'quiet';

// Drives the phase and the one timer that moves playing to quiet.
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
