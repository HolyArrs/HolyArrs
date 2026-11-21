import { describe, expect, it } from 'vitest';
import { permWords } from './perm-words.ts';

describe('permission words', () => {
	it('names the YouTube and save-files permissions', () => {
		const w = permWords();
		expect(w.youtube).toBe('Use the YouTube library');
		expect(w.save_files).toBe('Save files to their device');
	});
});
