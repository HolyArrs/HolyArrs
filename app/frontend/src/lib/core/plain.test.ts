import { describe, expect, it } from 'vitest';
import { decided, healthSentence, looksLikeId, peopleNames, personName, plainReason } from './plain.ts';

describe('plain words', () => {
	it('spots internal ids', () => {
		for (const n of ['e2e-member-3', 'preview_admin', '1234567', '9f8e7d6c5b4a3f2e', '', null]) expect(looksLikeId(n)).toBe(true);
		for (const n of ['Ana', 'Luis Garcia', 'Zoë']) expect(looksLikeId(n)).toBe(false);
	});
	it('names people or says someone in the household', () => {
		expect(personName('Ana ')).toBe('Ana');
		expect(personName('e2e-alice')).toBe('Someone in your household');
		expect(personName(undefined, 'someone')).toBe('someone');
	});
	it('lists people, folding ids together', () => {
		expect(peopleNames(['Ana', 'Bo'])).toBe('Ana and Bo');
		expect(peopleNames(['e2e-1'])).toBe('Someone in your household');
		expect(peopleNames(['e2e-1', 'preview-2'])).toBe('2 people in your household');
		expect(peopleNames(['Ana', 'e2e-1'])).toBe('Ana and one other in your household');
	});
	it('moves technical text out of reasons', () => {
		const raw = 'The file could not be placed: GetFileAttributesEx C:\\Temp\\x.mkv: The system cannot find the file specified.';
		expect(plainReason(raw)).toEqual({ text: 'The file could not be placed.', detail: raw });
		expect(plainReason('The download failed and no other Release passed the Pick rules.')).toEqual({ text: 'The download failed and no other Release passed the Pick rules.' });
		expect(plainReason('open /data/x: permission denied').text).toBe('Something went wrong with the files.');
	});
	it('says health problems without raw text', () => {
		expect(healthSentence({ kind: 'library_folder', state: 'error' })).toBe('HolyArrs cannot reach a Library folder.');
		expect(healthSentence({ kind: 'disk_space', state: 'warning' })).toBe('A Library folder is running low on space.');
		expect(healthSentence({ kind: 'config_disk', state: 'error' })).toBe('The disk that holds the config folder is almost full.');
		expect(healthSentence({ kind: 'nope' as never, state: 'error' })).toBe('Something in HolyArrs is not working.');
	});
	it('says who decided an ask', () => {
		expect(decided({ state: 'asked', auto_approved: false })).toBe('');
		expect(decided({ state: 'approved', decided_by: { id: 1, name: 'luis' }, auto_approved: false })).toBe('Approved by luis');
		expect(decided({ state: 'available', decided_by: { id: 2, name: 'ana' }, auto_approved: true })).toBe('Auto-approved');
		expect(decided({ state: 'turned_down', decided_by: { id: 1, name: 'luis' }, auto_approved: false })).toBe('Declined by luis');
		expect(decided({ state: 'turned_down', decided_by: { id: 1, name: '' }, auto_approved: false })).toBe('Declined by an admin');
		expect(decided({ state: 'turned_down', decided_by: { id: 3, name: 'ana', display_name: 'Ana M' }, auto_approved: false }, { id: 1, displayName: 'Luis R' })).toBe('Declined by Ana M');
		expect(decided({ state: 'turned_down', decided_by: { id: 1, name: 'luis' }, auto_approved: false }, { id: 1, displayName: 'Luis R' })).toBe('Declined by Luis R');
		expect(decided({ state: 'turned_down', decided_by: { id: 1, name: 'luis' }, auto_approved: false }, { id: 2, displayName: 'Ana' })).toBe('Declined by luis');
	});
});
