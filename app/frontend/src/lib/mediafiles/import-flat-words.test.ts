import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { importKey, itemsAreFiles } from './adopt-parts.ts';

const read = (p: string) => readFileSync(new URL(p, import.meta.url), 'utf8');
const en: Record<string, string> = JSON.parse(read('../core/i18n/messages/en/library.json'));
const page = read('../../routes/library/import/+page.svelte');
const list = read('./ImportList.svelte');

const counted = ['foldersInOne', 'foldersInMany', 'foldersPart', 'noFolders', 'searchFolders', 'folderCountOne', 'folderCountMany', 'readCount', 'titleAndFolder', 'leftHint', 'askScanRestOne', 'askScanRestMany', 'leaveAsk', 'interrupted', 'interruptedLeft', 'interruptedSignedOut'] as const;

describe('Import words for a flat Library folder', () => {
	it('keeps the folder words for a Library folder that has folders', () => {
		for (const k of counted) expect(importKey(k, false)).toBe(`library.import.${k}`);
		expect(importKey('foldersInOne')).toBe('library.import.foldersInOne');
	});
	it('calls the items files, never folders, when the Library folder keeps movies as files', () => {
		for (const k of counted) {
			const key = importKey(k, true);
			expect(key).not.toBe(`library.import.${k}`);
			const text = en[key];
			expect(text, key).toBeTruthy();
			expect(text.replace('{path}', '').toLowerCase().replace('library folder', ''), key).not.toContain('folder');
			expect(text, key).toMatch(/file/i);
		}
	});
	it('takes the full key of a note too, so a cut-short import reads the same way', () => {
		expect(importKey('library.import.interruptedLeft', true)).toBe('library.import.flatInterruptedLeft');
		expect(importKey('library.import.interrupted', false)).toBe('library.import.interrupted');
	});
	it('calls a scan files when its Library folder is flat or every row is a loose file', () => {
		expect(itemsAreFiles({ flat: true, folders: [] })).toBe(true);
		expect(itemsAreFiles({ folders: [{ loose: true }, { loose: true }] })).toBe(true);
		expect(itemsAreFiles({ folders: [{ loose: true }, {}] })).toBe(false);
		expect(itemsAreFiles({ folders: [] })).toBe(false);
		expect(itemsAreFiles(null)).toBe(false);
	});
	it('names the column of a flat scan File', () => {
		expect(en['library.import.flatColPath']).toBe('File');
		expect(list).toContain("flat ? 'library.import.flatColPath' : 'mediafiles.col.path'");
		expect(page).toContain('itemsAreFiles(plan)');
	});
	it('reads the flat flag on the scan page and the list header', () => {
		expect(page).toContain('importKey(');
		expect(page).not.toMatch(/t\('library\.import\.(noFolders|searchFolders|readCount)'/);
		expect(page).toContain("t(importKey('leaveAsk', flat))");
		expect(page).toContain("importKey(picks.length === 1 ? 'askScanRestOne' : 'askScanRestMany', flat)");
		expect(page).toContain("t(importKey('leftHint', flat))");
		expect(page).toContain('t(importKey(cut.key, cutFlat)');
		expect(list).toContain("importKey('titleAndFolder', flat)");
	});
});
