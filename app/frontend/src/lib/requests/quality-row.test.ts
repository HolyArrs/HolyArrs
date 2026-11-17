import { describe, expect, it } from 'vitest';
import { memberRow, ownerRow, qualityName, qualityTag } from './quality-row.ts';
import type { QualityChoice } from './types.ts';

const std: QualityChoice = { id: 'standard', name: '', extra: false, pick_rules_id: 0 };
const uhd: QualityChoice = { id: '4k', name: '', extra: true, pick_rules_id: 9 };

describe('the Quality row on a request sheet', () => {
	it('shows Standard and 4K in the owner order, the first one picked', () => {
		const r = memberRow([std, uhd]);
		expect(r.shown).toBe(true);
		expect(r.pills.map((p) => [p.label, p.disabled])).toEqual([['Standard', false], ['4K', false]]);
		expect(r.selected).toBe('standard');
	});

	it('is hidden when there is only one choice (4K not set up, hidden, or a Kid)', () => {
		const r = memberRow([std]);
		expect(r.shown).toBe(false);
		expect(r.selected).toBe('standard');
	});

	it('uses the owner names', () => {
		expect(memberRow([{ ...std, name: 'HD' }, { ...uhd, name: 'UHD' }]).pills.map((p) => p.label)).toEqual(['HD', 'UHD']);
	});

	it('turns off a line whose copy is in the Library and picks the next one', () => {
		const r = memberRow([std, uhd], { inLibrary: true });
		expect(r.pills[0]).toMatchObject({ disabled: true, note: 'In library' });
		expect(r.pills[1].disabled).toBe(false);
		expect(r.selected).toBe('4k');
		const both = memberRow([std, uhd], { inLibrary: true, has4k: true });
		expect(both.pills.every((p) => p.disabled)).toBe(true);
		expect(both.selected).toBe('');
	});

	it('turns off a line the person asked for already', () => {
		const r = memberRow([std, uhd], { mine: ['standard'] });
		expect(r.pills[0]).toMatchObject({ disabled: true, note: 'Requested' });
		expect(r.selected).toBe('4k');
	});

	it('starts on a line someone else asked for, so asking joins their request', () => {
		expect(memberRow([std, uhd], { open: ['4k'] }).selected).toBe('4k');
		expect(memberRow([std, uhd], { open: ['standard', '4k'] }).selected).toBe('standard');
	});

	it('starts on the preferred line when it can be asked', () => {
		expect(memberRow([std, uhd], { prefer: '4k' }).selected).toBe('4k');
		expect(memberRow([std, uhd], { prefer: '4k', has4k: true }).selected).toBe('standard');
	});
});

describe('the Quality row when approving', () => {
	const rules = [
		{ id: 3, name: 'Lite' },
		{ id: 9, name: '4K' },
		{ id: 5, name: 'Best' }
	];

	it('lists the main lines, the other rules, then the extra copy, with what was asked picked', () => {
		const r = ownerRow([std, uhd], rules, { quality: '4k', four_k: true });
		expect(r.pills.map((p) => p.label)).toEqual(['Standard', 'Lite', 'Best', '4K']);
		expect(r.asked).toBe('4k');
		expect(r.pills.find((p) => p.label === 'Lite')).toMatchObject({ quality: 'standard', rulesId: 3 });
		expect(r.pills.find((p) => p.label === '4K')).toMatchObject({ quality: '4k', rulesId: 0 });
	});

	it('has no Default button and does not show the Standard rules twice', () => {
		const r = ownerRow([{ ...std, pick_rules_id: 3 }, uhd], rules, { quality: 'standard' });
		expect(r.pills.map((p) => p.label)).toEqual(['Standard', 'Best', '4K']);
	});

	it('shows a rule named like a line once', () => {
		const r = ownerRow([std, uhd], [{ id: 1, name: 'Standard' }, ...rules], { quality: 'standard' });
		expect(r.pills.map((p) => p.label)).toEqual(['Standard', 'Lite', 'Best', '4K']);
	});

	it('keeps the asked line when it is hidden now, and reads older answers by four_k', () => {
		const r = ownerRow([std], [], { four_k: true });
		expect(r.asked).toBe('4k');
		expect(r.pills.map((p) => p.label)).toEqual(['Standard', '4K']);
	});
});

describe('the quality tag on request cards', () => {
	it('shows only when the line is not the first one', () => {
		expect(qualityTag({ kind: 'movie', quality: 'standard' })).toBe('');
		expect(qualityTag({ kind: 'movie', quality: '4k' })).toBe('4K');
		expect(qualityTag({ kind: 'show', quality: '4k', quality_name: 'UHD' })).toBe('UHD');
		expect(qualityTag({ kind: 'movie', four_k: true })).toBe('4K');
		expect(qualityTag({ kind: 'artist', quality: '' })).toBe('');
	});

	it('names the built-in lines', () => {
		expect(qualityName('standard')).toBe('Standard');
		expect(qualityName('4k')).toBe('4K');
		expect(qualityName('4k', 'Ultra')).toBe('Ultra');
	});
});
