import { describe, expect, it } from 'vitest';
import { leftInDownloader, needsFolder, standInFolder } from './hands-off.ts';

describe('hands-off mode', () => {
	it('reads a finished hands-off download as left in the Downloader', () => {
		expect(leftInDownloader({ state: 'in_library' })).toBe(true);
		expect(leftInDownloader({ state: 'in_library', file: { Size: 1 } })).toBe(false);
		expect(leftInDownloader({ state: 'downloading' })).toBe(false);
		expect(leftInDownloader({ state: 'wanted' })).toBe(false);
	});

	it('knows the stand-in folder is not a real one', () => {
		expect(standInFolder('hands-off:movie/Dune - Part Two (2024)')).toBe(true);
		expect(standInFolder('/movies/Dune - Part Two (2024)')).toBe(false);
		expect(standInFolder(undefined)).toBe(false);
	});

	it('lets the add sheets add with no folder only in hands-off mode', () => {
		expect(needsFolder(0, false)).toBe(true);
		expect(needsFolder(0, true)).toBe(false);
		expect(needsFolder(2, false)).toBe(false);
	});
});
