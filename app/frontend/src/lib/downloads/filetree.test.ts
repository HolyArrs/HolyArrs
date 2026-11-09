import { fileTree } from './filetree.ts';
import { describe, expect, it } from 'vitest';

describe('fileTree', () => {
	const files = [
		{ name: 'Show/S01/e1.mkv', size: 100, done: 50 },
		{ name: 'Show/S01/e2.mkv', size: 100, done: 0 },
		{ name: 'Show/info.nfo', size: 10, done: 10 }
	];
	it('adds folder rows that sum their files', () => {
		const t = fileTree(files, () => false);
		expect(t.map((r) => r.key)).toEqual(['Show', 'Show/S01', 'Show/S01/e1.mkv', 'Show/S01/e2.mkv', 'Show/info.nfo']);
		expect(t[0]).toMatchObject({ dir: true, size: 210, done: 60 });
		expect(t[2]).toMatchObject({ dir: false, i: 0, depth: 2 });
	});
	it('hides what sits in a closed folder', () => {
		const t = fileTree(files, (k) => k === 'Show/S01');
		expect(t.map((r) => r.key)).toEqual(['Show', 'Show/S01', 'Show/info.nfo']);
	});
	it('a single file has no folder row', () => {
		expect(fileTree([{ name: 'a.mkv', size: 1, done: 1 }], () => false)).toHaveLength(1);
	});
});
