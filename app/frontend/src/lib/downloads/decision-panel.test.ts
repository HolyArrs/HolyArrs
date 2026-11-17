import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { lang } from '#lib/core/i18n/i18n.svelte.ts';
import { plainReason } from '../core/plain.ts';
import { reasonWords } from './decision-fix.ts';

const src = readFileSync(new URL('./DecisionPanel.svelte', import.meta.url), 'utf8');

describe('decision panel', () => {
	// plainReason ends the line with a full stop the stored lines lack, so the Why? box reads it the way the card line does.
	it('shows the Why? line in the picked language', () => {
		const box = src.split('data-testid="decision-why-box"')[1]?.split('</div>')[0] ?? '';
		expect(box).toContain('reasonWords(r.text)');
		expect(box).not.toContain('serverWords(r.text)');
		const de = Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('../core/i18n/messages/de/server-lines.json', { eager: true })).map((f) => f.default));
		const start = { value: lang.value, words: lang.words };
		lang.value = 'de';
		lang.words = de;
		expect(reasonWords(plainReason('The file could not be placed: lstat /x: no such file or directory.').text)).toBe('Die Datei konnte nicht abgelegt werden.');
		// What Import again answers when nothing waits, and the card of a download that is gone.
		expect(reasonWords(plainReason('this download does not wait for an import').text)).toBe('Dieser Download wartet nicht auf einen Import');
		expect(reasonWords(plainReason('the download HolyArrs was importing is gone; search again or import the files by hand').text)).toBe('Der Download, den HolyArrs gerade importierte, ist weg; suche erneut oder importiere die Dateien von Hand');
		lang.value = start.value;
		lang.words = start.words;
	});
	it('asks for the import of the same download again, in front and in More choices', () => {
		const fn = src.split('async function importAgain')[1]?.split('\n\t}\n')[0] ?? '';
		expect(fn).toContain('api.post(`/downloads/${d.download_id}/import`)');
		expect(fn).toContain('await refreshDecisions()');
		expect(src).toContain("{:else if fix === 'again'}");
		expect(src.split('function more(')[1]?.split('\n\t}\n')[0] ?? '').toContain("canImportAgain(d) && fix !== 'again'");
	});
	it('shows every likely-a-fake line in German, with no English part', () => {
		const de = Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('../core/i18n/messages/de/server-lines.json', { eager: true })).map((f) => f.default));
		const start = { value: lang.value, words: lang.words };
		lang.value = 'de';
		lang.words = de;
		const lines: [string, string][] = [
			['the file is empty; likely a fake', 'Die Datei ist leer; wahrscheinlich eine Fälschung'],
			['the file is 12 KB, too small to be a track; likely a fake', 'Die Datei hat 12 KB, zu klein für einen Titel; wahrscheinlich eine Fälschung'],
			['the file is 40 MB, too small to be the full video; likely a fake', 'Die Datei hat 40 MB, zu klein für das ganze Video; wahrscheinlich eine Fälschung'],
			['a .wmv file where the Release said 1080p; likely a fake', 'Eine .wmv-Datei, obwohl das Release 1080p angab; wahrscheinlich eine Fälschung'],
			['the Release said 4.4 GB but holds 2 MB of video or audio; likely a fake', 'Das Release gab 4.4 GB an, enthält aber 2 MB Video oder Audio; wahrscheinlich eine Fälschung'],
			['the file could not be placed: the file is empty; likely a fake', 'Die Datei konnte nicht abgelegt werden: die Datei ist leer; wahrscheinlich eine Fälschung']
		];
		for (const [en, want] of lines) {
			const got = reasonWords(en);
			expect(got).toBe(want);
			expect(got).not.toMatch(/likely|fake|file is|too small|holds/i);
		}
		lang.value = start.value;
		lang.words = start.words;
	});
	it('leads the album card with Import again for the whole download, and keeps Search again for these under More', () => {
		const pack = src.split('data-testid="decision-pack"')[1]?.split('<ul class="subs">')[0] ?? '';
		expect(src).toContain('{@const again = packImportAgain(g.items)}');
		expect(pack).toMatch(/\{#if again\}\s*<Button[^>]*onclick=\{\(\) => importAgain\(d, g\.key\)\}/);
		expect(pack).toContain("t('downloads.dp.searchThese'");
		expect(pack).toContain('packMore(d, g.key, again)');
		const more = src.split('function packMore(')[1]?.split('\n\t}\n')[0] ?? '';
		expect(more).toContain("again ? [{ label: t('downloads.dp.searchThese'");
	});
	it('reads the cards again when Import this file fails, so the new reason shows at once', () => {
		const fn = src.split('async function importFile')[1]?.split('\n\t}\n')[0] ?? '';
		const caught = fn.split('} catch (e) {')[1] ?? '';
		expect(caught).toContain('await refreshDecisions()');
		expect(caught).toContain('reasonWords(plainReason(message(e)).text)');
	});
});
