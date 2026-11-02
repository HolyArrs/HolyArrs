// Source health in words and tones, told apart by why a Source waits.
import type { SourceHealth } from './types.ts';
import type { Tone } from './format.ts';
import { healthWords, sourceTone } from './sources.ts';

export const reasonWords: Record<NonNullable<SourceHealth['reason']>, string> = {
	slowed: 'Slowed down',
	down: 'Down',
	blocked: 'Blocked',
	key: 'Key refused'
};

const reasonTone: Record<NonNullable<SourceHealth['reason']>, Tone> = { slowed: 'wait', down: 'fail', blocked: 'fail', key: 'fail' };

/** healthWord is the chip word: the reason when it waits, else the state. */
export function healthWord(h: Pick<SourceHealth, 'state' | 'reason'>): string {
	return (h.reason && reasonWords[h.reason]) || healthWords[h.state] || h.state;
}

/** healthTone colours the chip: slowed down waits, other reasons fail. */
export function healthTone(h: Pick<SourceHealth, 'state' | 'reason'>): Tone {
	return (h.reason && reasonTone[h.reason]) || sourceTone(h.state);
}

/** pausedUntil says when HolyArrs tries again on its own, or ''. */
export function pausedUntil(h: Pick<SourceHealth, 'until'>): string {
	if (!h.until) return '';
	const t = new Date(h.until);
	return Number.isNaN(t.getTime()) ? '' : `Paused until ${t.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}`;
}
