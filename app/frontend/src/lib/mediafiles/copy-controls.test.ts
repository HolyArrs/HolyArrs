import { describe, expect, it } from 'vitest';
import { copyEdition, editionDecisionBody, inspectedAudio, languageList, versionCriteria, type CopyControlFile } from './copy-controls.ts';

describe('copy controls', () => {
	it('keeps inspected audio separate from the Version criteria', () => {
		const file: CopyControlFile = {
			ID: 7,
			VersionID: 2,
			RelativePath: 'Arrival/Arrival.mkv',
			MediaInfo: { audio_languages: ['fr', 'en', 'fr'], edition: 'Director cut' }
		};
		expect(inspectedAudio(file)).toEqual(['fr', 'en']);
		expect(copyEdition(file)).toBe('Director cut');
		expect(versionCriteria({ edition: 'Theatrical', audio_languages: ['en', 'de'] })).toEqual(['Edition: Theatrical', 'Required audio: en, de']);
	});

	it('normalises typed language criteria without inventing values', () => {
		expect(languageList(' en, FR, en ,, de ')).toEqual(['en', 'fr', 'de']);
		expect(inspectedAudio({ ID: 1, VersionID: 1, RelativePath: 'x.mkv', MediaInfo: { audio_languages: 'English' } })).toEqual([]);
		expect(versionCriteria({})).toEqual(['Any edition', 'Any audio language']);
	});

	it('retries an edition decision only with an explicit confirmation', () => {
		expect(editionDecisionBody(' Director cut ')).toEqual({ edition: 'Director cut', confirm: true, retry: true });
	});
});
