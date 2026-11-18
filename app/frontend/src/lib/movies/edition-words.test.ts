import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const pack = (lang: string) => JSON.parse(readFileSync(new URL(`../core/i18n/messages/${lang}/movies.json`, import.meta.url), 'utf8')) as Record<string, string>;

// Per language: the word its edition field uses, and the word its Version name field uses.
const words: Record<string, [edition: string, version: string]> = {
	ar: ['الإصدار', 'النسخة'],
	bn: ['এডিশন', 'সংস্করণ'],
	cs: ['edic', 'vydání'],
	fa: ['ویرایش', 'نسخه'],
	hi: ['एडिशन', 'संस्करण'],
	ja: ['版', 'エディション'],
	pl: ['wydani', 'edycj'],
	ru: ['версию фильма', 'издание'],
	sk: ['edíci', 'vydanie'],
	ta: ['வெளியீட்டு வடிவ', 'பதிப்ப'],
	tr: ['baskı', 'sürüm'],
	'zh-cn': ['剪辑版', '版本'],
	'zh-tw': ['剪輯版', '版本']
};

describe('edition messages name the edition field, not the Version', () => {
	for (const [lang, [edition, version]] of Object.entries(words)) {
		it(`${lang}: the refusal line uses the edition field's word`, () => {
			const line = pack(lang)['movies.versions.editionInvalid'].toLowerCase();
			expect(line).toContain(edition);
			expect(line).not.toContain(version);
		});
	}

	it('the per-file hint does not call the edition a Version', () => {
		for (const [lang, version] of [['ar', 'النسخة'], ['cs', 'verze'], ['da', 'version'], ['fr', 'version']]) expect(pack(lang)['movies.versions.editionFileHint'].toLowerCase()).not.toContain(version);
	});

	it('Chinese and Japanese lines use full-width brackets and commas', () => {
		for (const lang of ['zh-cn', 'zh-tw', 'ja']) expect(pack(lang)['movies.versions.editionInvalid']).not.toMatch(/[(),]/);
	});
});
