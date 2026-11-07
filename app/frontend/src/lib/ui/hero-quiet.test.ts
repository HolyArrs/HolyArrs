import { describe, expect, it } from 'vitest';
import { heroQuiet, heroStep, QUIET_AFTER, wordsShown, type HeroPhase } from './hero-quiet.ts';

function fake() {
	let now = 0;
	let id = 0;
	const jobs = new Map<number, { at: number; f: () => void }>();
	return {
		timers: {
			set: (f: () => void, ms: number) => (jobs.set(++id, { at: now + ms, f }), id as unknown as ReturnType<typeof setTimeout>),
			clear: (t: ReturnType<typeof setTimeout>) => void jobs.delete(t as unknown as number)
		},
		tick(ms: number) {
			now += ms;
			for (const [k, j] of [...jobs]) if (j.at <= now) (jobs.delete(k), j.f());
		},
		get pending() { return jobs.size; }
	};
}

describe('heroStep', () => {
	it('folds only from playing and stays folded for the title', () => {
		expect(heroStep('still', 'settle')).toBe('still');
		expect(heroStep('playing', 'settle')).toBe('quiet');
		expect(heroStep('quiet', 'stop')).toBe('quiet');
		expect(heroStep('playing', 'stop')).toBe('ended');
		expect(heroStep('ended', 'play')).toBe('playing');
		expect(heroStep('quiet', 'play')).toBe('quiet');
		expect(heroStep('quiet', 'reset')).toBe('still');
		expect(wordsShown('quiet')).toBe(false);
		for (const p of ['still', 'playing', 'ended'] as HeroPhase[]) expect(wordsShown(p)).toBe(true);
	});
});

describe('heroQuiet', () => {
	it('runs the measured sequence: play, fold after the wait, stays folded on end', () => {
		const f = fake();
		const seen: HeroPhase[] = [];
		const h = heroQuiet((p) => seen.push(p), f.timers);
		h.send('play');
		f.tick(QUIET_AFTER - 1);
		expect(h.phase).toBe('playing');
		f.tick(1);
		expect(h.phase).toBe('quiet');
		h.send('stop');
		h.send('play');
		expect(h.phase).toBe('quiet');
		expect(seen).toEqual(['playing', 'quiet']);
	});

	it('never sticks: a stop before the wait cancels the fold', () => {
		const f = fake();
		const h = heroQuiet(() => {}, f.timers);
		h.send('play');
		f.tick(1000);
		h.send('stop');
		f.tick(QUIET_AFTER * 2);
		expect(h.phase).toBe('ended');
		expect(f.pending).toBe(0);
	});

	it('survives many plays and stops in a row (hover storms)', () => {
		const f = fake();
		const h = heroQuiet(() => {}, f.timers);
		for (let i = 0; i < 50; i++) {
			h.send('play');
			f.tick(QUIET_AFTER);
			expect(h.phase).toBe('quiet');
			h.send('play');
			expect(h.phase).toBe('quiet');
			h.send('stop');
			expect(wordsShown(h.phase)).toBe(false);
			h.send('reset');
		}
		expect(f.pending).toBe(0);
	});

	it('reset shows the words and drops the timer', () => {
		const f = fake();
		const h = heroQuiet(() => {}, f.timers);
		h.send('play');
		h.send('reset');
		f.tick(QUIET_AFTER);
		expect(h.phase).toBe('still');
	});
});
