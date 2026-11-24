import { describe, expect, it } from 'vitest';
import { contentNotes, CONTENT_NOTES } from './content-notes.ts';

describe('content notes', () => {
	const cases: { name: string; raw: unknown; want: string[] }[] = [
		{ name: 'missing', raw: undefined, want: [] },
		{ name: 'not a list', raw: 'nudity', want: [] },
		{ name: 'empty', raw: [], want: [] },
		{ name: 'known notes in the order given', raw: ['nudity', 'animal_death'], want: ['Nudity', 'An animal dies'] },
		{ name: 'unknown and odd entries dropped', raw: ['nudity', 'spoilers', 3, null, ''], want: ['Nudity'] },
		{ name: 'repeats once', raw: ['drugs', 'drugs'], want: ['Drug use'] },
		{ name: 'object keys are not notes', raw: ['constructor', 'toString'], want: [] }
	];
	for (const c of cases) it(c.name, () => expect(contentNotes(c.raw)).toEqual(c.want));
	it('has a word for every note the server sends', () => {
		expect(Object.keys(CONTENT_NOTES)).toEqual(['nudity', 'sex', 'sexual_violence', 'gore', 'animal_death', 'suicide', 'drugs', 'child_abuse', 'domestic_abuse']);
	});
});
