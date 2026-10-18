import { describe, expect, it } from 'vitest';
import { healthSentence, looksLikeId, peopleNames, personName } from './plain.ts';

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
	it('says health problems without raw text', () => {
		expect(healthSentence({ kind: 'library_folder', state: 'error' })).toBe('HolyArrs cannot reach a Library folder.');
		expect(healthSentence({ kind: 'disk_space', state: 'warning' })).toBe('A Library folder is running low on space.');
		expect(healthSentence({ kind: 'nope' as never, state: 'error' })).toBe('Something in HolyArrs is not working.');
	});
});
