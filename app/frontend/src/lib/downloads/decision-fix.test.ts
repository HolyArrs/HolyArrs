import { decisionFix, noMatchKind, reasonKind } from './decision-fix.ts';
import { describe, expect, it } from 'vitest';

const dec = (o: Record<string, unknown>) => ({ kind: 'movie', why: 'conflict', download_id: 7, reason: '', ...o }) as Parameters<typeof decisionFix>[0];

describe('decision fix', () => {
	it('sends a missing save folder to Settings > Downloaders', () => {
		const reason = "HolyArrs can't read this download's files: HOLYARRS_SAVE_PATH is not set and SABnzbd has no path pair. Set HOLYARRS_SAVE_PATH if both apps see the same download folder, or add a path pair in Settings > Downloaders, then import again";
		expect(decisionFix(dec({ reason }))).toBe('folder');
		expect(decisionFix(dec({ kind: 'show', why: 'unreadable', reason }))).toBe('folder');
		expect(reasonKind(dec({ reason }))).toBe('plain');
	});
	it('offers Set edition only for an edition refusal', () => {
		for (const reason of ['edition is unknown; confirm the edition or choose an unconstrained Version', 'edition is ambiguous; confirm it explicitly', 'edition does not match the selected Version', 'file and release name disagree about the edition; confirm it explicitly']) {
			expect(decisionFix(dec({ reason }))).toBe('edition');
			expect(reasonKind(dec({ reason }))).toBe('edition');
		}
	});
	it('never offers Set edition for another conflict', () => {
		expect(decisionFix(dec({ reason: 'No file in the download matched this movie; match the files by hand.' }))).toBe('hand');
		expect(reasonKind(dec({ reason: 'No file in the download matched this movie; match the files by hand.' }))).toBe('nomatch');
		expect(reasonKind(dec({ reason: 'nothing could be placed: not enough free space in the Library folder' }))).toBe('plain');
	});
	it('points a copy in place reason to another Version', () => {
		for (const reason of ['the copy in place is protected; choose another Version for an upgrade', 'the new edition differs from the copy in place; choose another Version', "file is 1080p web, which this Version's Pick rules don't allow, so it can't replace the copy in place; choose another Version"]) {
			expect(decisionFix(dec({ reason }))).toBe('version');
			expect(reasonKind(dec({ reason }))).toBe('copy');
		}
	});
	it('leads with Import this file when the movie download holds one video', () => {
		const reason = 'No file in the download matched this movie; match the files by hand.';
		expect(decisionFix(dec({ reason, import_file: 'grp-ean.mkv' }))).toBe('import');
		expect(reasonKind(dec({ reason, import_file: 'grp-ean.mkv' }))).toBe('nomatch');
		expect(decisionFix(dec({ reason: 'edition is unknown', import_file: 'x.mkv' }))).toBe('edition');
		expect(decisionFix(dec({ kind: 'show', reason, import_file: 'x.mkv' }))).toBe('hand');
	});
	it('leads with Import files by hand when nothing matched and the folder is known (#312)', () => {
		const reason = 'No file in the download matched an episode; match the files by hand.';
		expect(decisionFix(dec({ kind: 'show', reason, import_folder: '/data/x' }))).toBe('match');
		expect(decisionFix(dec({ kind: 'show', reason }))).toBe('hand');
		expect(decisionFix(dec({ reason: 'nothing could be placed; look at the import plan', import_folder: '/d' }))).toBe('match');
		expect(decisionFix(dec({ reason: 'No file in the download matched this movie; match the files by hand.', import_folder: '/d', import_file: 'a.mkv' }))).toBe('import');
		expect(reasonKind(dec({ kind: 'show', reason, import_folder: '/d' }))).toBe('nomatch');
	});
	it('names what did not match, old reasons too', () => {
		expect(noMatchKind('No file in the download matched this movie; match the files by hand.')).toBe('movie');
		expect(noMatchKind('no file in the download matched an episode; match the files by hand')).toBe('episode');
		expect(noMatchKind('No file in the download matched a track; match the files by hand.')).toBe('track');
		expect(noMatchKind('Nothing could be placed; look at the import plan.')).toBe('movie');
		expect(noMatchKind('nothing in the download could be matched to an episode; look at the import plan')).toBe('episode');
		expect(noMatchKind('nothing in the download could be matched to a track; look at the import plan')).toBe('track');
		expect(noMatchKind('nothing could be placed: not enough free space in the Library folder')).toBe('');
	});
	it('keeps shows and missing downloads off the movie fixes', () => {
		expect(decisionFix(dec({ kind: 'show', reason: 'edition is unknown' }))).toBe('hand');
		expect(decisionFix(dec({ download_id: undefined, reason: 'edition is unknown' }))).toBe('hand');
		expect(decisionFix(dec({ kind: 'show', reason: 'the copy in place is protected; choose another Version for an upgrade' }))).toBe('hand');
		expect(decisionFix(dec({ why: 'ran_out', reason: 'no other Release passed the Pick rules' }))).toBe('search');
		expect(reasonKind(dec({ why: 'taken', reason: 'x' }))).toBe('taken');
	});
});
