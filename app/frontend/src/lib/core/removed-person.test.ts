import { afterEach, describe, expect, it } from 'vitest';
import { lang } from './i18n/i18n.svelte.ts';
import { shownPerson, decidedWho, peopleNames, personParts, personText, bareNames, isGone } from './plain.ts';
import { byWhom } from '#lib/downloads/history-kinds.ts';
import { textMatches } from '#lib/downloads/history-search.ts';
import { askedByLine, firstName, initial } from '#lib/ui/asked-by.ts';
import { linkedPeople } from '#lib/mediafiles/library-view.ts';
import { whoName } from '#lib/requests/reports.ts';
import { askedBy } from '#lib/requests/ask-words.ts';
import { askers } from '#lib/requests/inbox.ts';

const start = { value: lang.value, words: lang.words };
afterEach(() => {
	lang.value = start.value;
	lang.words = start.words;
});
const de = Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('./i18n/messages/de/*.json', { eager: true })).map((f) => f.default));
function german() {
	lang.value = 'de';
	lang.words = { ...lang.words, ...de };
}

// How the server stores a deleted person: their real name, an invisible separator, their id.
const stored = 'maria Lopez⁣5';
const said = (s: string) => personText(s);

describe('a deleted person keeps their real name, marked deleted', () => {
	it('keeps the name and marks it', () => {
		expect(isGone(stored)).toBe(true);
		expect(isGone('ana')).toBe(false);
		expect(shownPerson('ana')).toBe('ana');
		expect(bareNames(shownPerson(stored))).toBe('maria Lopez');
		expect(personParts(`Asked by ${shownPerson(stored)} and ana`)).toEqual([
			{ text: 'Asked by ', gone: false },
			{ text: 'maria Lopez', gone: true },
			{ text: ' and ana', gone: false }
		]);
		expect(personText(stored)).toBe('maria Lopez (deleted)');
	});
	it('reads out in the picked language', () => {
		german();
		expect(personText(stored)).toBe('maria Lopez (gelöscht)');
	});
	it('a name someone typed with the separator but no id is not marked', () => {
		expect(isGone('maria⁣')).toBe(false);
		expect(personParts('maria')).toEqual([{ text: 'maria', gone: false }]);
	});
	it('on request cards, filters and decisions', () => {
		const gone = { id: 5, name: stored };
		expect(said(askedBy({ people: [], asked_by: gone } as never))).toContain('maria Lopez (deleted)');
		expect(said(askers([{ people: [], asked_by: gone }] as never)[0].name)).toBe('maria Lopez (deleted)');
		expect(said(decidedWho({ state: 'approved', decided_by: gone } as never))).toBe('maria Lopez (deleted)');
	});
	it('in name lists, History lines and History search', () => {
		expect(said(peopleNames(['ana', stored]))).toContain('maria Lopez (deleted)');
		const e = { Actor: { kind: 'user', id: 5 }, Data: {}, actor_name: stored, Kind: 'x', Summary: '' };
		expect(said(byWhom(e as never))).toBe('maria Lopez (deleted)');
		expect(textMatches(e as never, 'maria')).toBe(true);
	});
	it('in the Asked by byline and the Library people filter', () => {
		expect(said(firstName(stored))).toBe('maria Lopez (deleted)');
		expect(firstName('Ana Lopez')).toBe('Ana');
		expect(initial(stored)).toBe('M');
		expect(said(askedByLine([{ id: 5, name: stored, asked_at: '', me: false }]))).toContain('maria Lopez (deleted)');
		const gone = { id: 5, name: stored };
		expect(said(linkedPeople({ id: 1, asks: [{ title_id: 1, asked_by: gone, people: [] }] } as never)[0].name)).toBe('maria Lopez (deleted)');
	});
	it('on problem reports', () => {
		expect(said(whoName({ id: 5, name: stored, display_name: '' } as never))).toBe('maria Lopez (deleted)');
	});
});
