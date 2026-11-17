import { describe, expect, it } from 'vitest';
import { LINES_MAX, addLine, moveLine, ownLine, ownerName, removeLine } from './quality-lines.ts';
import { memberRow } from './quality-row.ts';
import type { QualityLine } from './types.ts';

const line = (id: string, more: Partial<QualityLine> = {}): QualityLine => ({ id, name: '', label: '', hidden: false, extra: id === '4k', pick_rules_id: 0, library_folder_id: 0, ready: true, ...more });
const base = [line('standard'), line('4k')];

describe('editing what people can ask for', () => {
	it('adds a line of the owner at the end, on the given quality, with an id of its own', () => {
		const one = addLine(base, 7);
		expect(one.map((l) => l.id)).toEqual(['standard', '4k', 'own-1']);
		expect(one[2]).toMatchObject({ name: '', pick_rules_id: 7, extra: false, hidden: false });
		expect(addLine(one, 7).map((l) => l.id)).toEqual(['standard', '4k', 'own-1', 'own-2']);
		expect(ownLine('own-1')).toBe(true);
		expect(ownLine('standard')).toBe(false);
	});

	it('stops adding at the top size', () => {
		let ls = base;
		for (let i = 0; i < 20; i++) ls = addLine(ls, 7);
		expect(ls).toHaveLength(LINES_MAX);
	});

	it('moves a line up or down and stays put at the ends', () => {
		expect(moveLine(base, 1, -1).map((l) => l.id)).toEqual(['4k', 'standard']);
		expect(moveLine(base, 0, 1).map((l) => l.id)).toEqual(['4k', 'standard']);
		expect(moveLine(base, 0, -1)).toBe(base);
		expect(moveLine(base, 1, 1)).toBe(base);
	});

	it('removes only a line the owner added', () => {
		const ls = addLine(base, 7);
		expect(removeLine(ls, 2).map((l) => l.id)).toEqual(['standard', '4k']);
		expect(removeLine(ls, 0)).toBe(ls);
	});
});

describe('a line of the owner on the request sheet', () => {
	const std = { id: 'standard', name: '', extra: false, pick_rules_id: 0 };
	const small = { id: 'own-1', name: 'Small', extra: false, pick_rules_id: 7 };
	const uhd = { id: '4k', name: '', extra: true, pick_rules_id: 9 };

	it('is a button with the owner name', () => {
		expect(memberRow([std, small, uhd]).pills.map((p) => p.label)).toEqual(['Standard', 'Small', '4K']);
	});

	it('is off with the other main-copy lines once the person asked for one of them', () => {
		const r = memberRow([std, small, uhd], { mine: ['standard'] });
		expect(r.pills.map((p) => p.disabled)).toEqual([true, true, false]);
		expect(r.selected).toBe('4k');
	});

	it('is off when the main copy is in the Library', () => {
		expect(memberRow([std, small, uhd], { inLibrary: true }).pills.map((p) => p.disabled)).toEqual([true, true, false]);
	});
});

describe('the owner name of a line', () => {
	const mk = (id: string, name = '', label = '') => ({ id, name, label, hidden: false, extra: false, pick_rules_id: 1, library_folder_id: 0, ready: true });
	it('is the owner label, else the name people see, else the built-in name', () => {
		expect(ownerName(mk('own-1', 'Small', 'Everyday 720p'))).toBe('Everyday 720p');
		expect(ownerName(mk('own-1', 'Small', '  '))).toBe('Small');
		expect(ownerName(mk('4k'))).toBe('4K');
	});
	it('a new line starts with no owner label', () => {
		expect(addLine([], 3)[0].label).toBe('');
	});
});
