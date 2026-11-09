import { describe, expect, it } from 'vitest';
import { askedByDated, askedByLine, askedByTip, firstName, shortDate, type Asker } from './asked-by.ts';

const at = '2026-10-12T15:00:00Z';
const p = (id: number, name: string, me = false): Asker => ({ id, name, asked_at: at, me });

describe('asked by wording', () => {
	it('says You asked, one, two and many askers', () => {
		expect(askedByLine([])).toBe('');
		expect(askedByLine([p(1, 'Luis', true)])).toBe('You asked');
		expect(askedByLine([p(2, 'Ana Lopez')])).toBe('Asked by Ana');
		expect(askedByLine([p(2, 'Ana'), p(3, 'Leo')])).toBe('Asked by Ana and Leo');
		expect(askedByLine([p(2, 'Ana'), p(3, 'Leo'), p(4, 'Mia')])).toBe('Asked by Ana and 2 others');
		expect(askedByLine([p(1, 'Luis', true), p(2, 'Ana')])).toBe('You and 1 other asked');
	});

	it('dates the title page line and the tooltip', () => {
		expect(shortDate(at)).toBe('12 Oct');
		expect(askedByDated([p(2, 'Ana')])).toBe('Asked by Ana · 12 Oct');
		expect(askedByTip([p(2, 'Ana')])).toBe('Ana asked for this on 12 Oct.');
		expect(askedByTip([p(1, 'Luis', true)])).toBe('You asked for this on 12 Oct.');
		expect(askedByTip([p(1, 'Bo', true), p(2, 'Ana')])).toBe('Ana asked for this on 12 Oct.');
	});

	it('uses first names only', () => {
		expect(firstName('  Leo  Diaz ')).toBe('Leo');
	});
});
