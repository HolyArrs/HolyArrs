import { describe, expect, it } from 'vitest';
import { fillWords, splitWords } from './words.ts';

describe('splitWords', () => {
	it('keeps text and marked parts in order', () => {
		expect(splitWords('Press <b>Make a key</b> first, see <a>API keys</a>.')).toEqual([
			{ tag: null, text: 'Press ' },
			{ tag: 'b', text: 'Make a key' },
			{ tag: null, text: ' first, see ' },
			{ tag: 'a', text: 'API keys' },
			{ tag: null, text: '.' }
		]);
	});

	it('leaves unknown tags as plain text', () => {
		expect(splitWords('backups/before-restore-<time>.')).toEqual([{ tag: null, text: 'backups/before-restore-<time>.' }]);
	});

	it('fills values after splitting, so a value never adds markers', () => {
		const parts = splitWords('Type <b>{name}</b> to confirm').map((p) => ({ ...p, text: fillWords(p.text, { name: '<b>x</b>' }) }));
		expect(parts[1]).toEqual({ tag: 'b', text: '<b>x</b>' });
	});
});
