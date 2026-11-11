import { decisionFix, reasonKind } from './decision-fix.ts';
import { describe, expect, it } from 'vitest';

const dec = (o: Record<string, unknown>) => ({ kind: 'movie', why: 'conflict', download_id: 7, reason: '', ...o }) as Parameters<typeof decisionFix>[0];

describe('decision fix (#274)', () => {
	it('offers Set edition only for an edition refusal', () => {
		for (const reason of ['edition is unknown; confirm the edition or choose an unconstrained Version', 'edition is ambiguous; confirm it explicitly', 'edition does not match the selected Version', 'file and release name disagree about the edition; confirm it explicitly']) {
			expect(decisionFix(dec({ reason }))).toBe('edition');
			expect(reasonKind(dec({ reason }))).toBe('edition');
		}
	});
	it('never offers Set edition for another conflict', () => {
		expect(decisionFix(dec({ reason: 'nothing could be placed; look at the import plan' }))).toBe('hand');
		expect(reasonKind(dec({ reason: 'nothing could be placed; look at the import plan' }))).toBe('plain');
	});
	it('points a copy in place reason to another Version', () => {
		for (const reason of ['the copy in place is protected; choose another Version for an upgrade', 'the new edition differs from the copy in place; choose another Version', "file is 1080p web, which this Version's Pick rules don't allow, so it can't replace the copy in place; choose another Version"]) {
			expect(decisionFix(dec({ reason }))).toBe('version');
			expect(reasonKind(dec({ reason }))).toBe('copy');
		}
	});
	it('keeps shows and missing downloads off the movie fixes', () => {
		expect(decisionFix(dec({ kind: 'show', reason: 'edition is unknown' }))).toBe('hand');
		expect(decisionFix(dec({ download_id: undefined, reason: 'edition is unknown' }))).toBe('hand');
		expect(decisionFix(dec({ kind: 'show', reason: 'the copy in place is protected; choose another Version for an upgrade' }))).toBe('hand');
		expect(decisionFix(dec({ why: 'ran_out', reason: 'no other Release passed the Pick rules' }))).toBe('search');
		expect(reasonKind(dec({ why: 'taken', reason: 'x' }))).toBe('taken');
	});
});
