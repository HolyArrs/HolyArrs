import { describe, expect, it } from 'vitest';
import { rematchAlbum, startPick, startTicked, type ByHandFile, type Pick } from './import-files.ts';

const track = (path: string, album: number, slot: number, confidence = 1): ByHandFile => ({
	path,
	name: path.split('/').pop()!,
	size: 10,
	kind: 'audio',
	guess: { title_id: 4, title_name: 'Ash Harbor', title_kind: 'artist', album_id: album, album_name: `Album ${album}`, slot_id: slot, track: slot, confidence }
});

describe('a new Pressing in Import files by hand (#777)', () => {
	it("re-matches only the album's files and keeps every other pick", () => {
		const before = [track('/m/a/01.flac', 9, 1), track('/m/a/12.flac', 9, 0, 0), track('/m/b/01.flac', 5, 50)];
		const picks: Record<string, Pick | null> = Object.fromEntries(before.map((f) => [f.path, startPick(f)]));
		const kept: Pick = { ...picks['/m/b/01.flac']!, slot_id: 51, track: 2 };
		picks['/m/b/01.flac'] = kept;
		const ticked: Record<string, boolean> = Object.fromEntries(before.map((f) => [f.path, startTicked(f, picks[f.path])]));
		ticked['/m/b/01.flac'] = false;
		// The deluxe Pressing has a twelfth track: the second file now finds it.
		const after = [track('/m/a/01.flac', 9, 101), track('/m/a/12.flac', 9, 112), track('/m/b/01.flac', 5, 50)];
		const r = rematchAlbum(after, picks, ticked, 9);
		expect(r.picks['/m/a/01.flac']?.slot_id).toBe(101);
		expect(r.picks['/m/a/12.flac']?.slot_id).toBe(112);
		expect(r.ticked['/m/a/12.flac']).toBe(true);
		expect(r.picks['/m/b/01.flac']).toEqual(kept);
		expect(r.ticked['/m/b/01.flac']).toBe(false);
	});

	it('drops a hand pick on the album, since its track may be gone', () => {
		const f = track('/m/a/03.flac', 9, 3);
		const handPick: Pick = { ...startPick(f)!, slot_id: 7, track: 7 };
		const r = rematchAlbum([track('/m/a/03.flac', 9, 103)], { [f.path]: handPick }, { [f.path]: true }, 9);
		expect(r.picks[f.path]?.slot_id).toBe(103);
	});
});

describe('a file no track of the new Pressing fits', () => {
	it('keeps its artist and album, drops the track and is unticked', () => {
		const f = track('/m/a/13.flac', 9, 13);
		const was = startPick(f)!;
		const r = rematchAlbum([track('/m/a/13.flac', 9, 0, 0)].map((x) => ({ ...x, guess: { ...x.guess, album_id: undefined, slot_id: undefined } })), { [f.path]: was }, { [f.path]: true }, 9);
		expect(r.picks[f.path]?.album_id).toBe(9);
		expect(r.picks[f.path]?.title_id).toBe(4);
		expect(r.picks[f.path]?.slot_id).toBeUndefined();
		expect(r.ticked[f.path]).toBe(false);
	});
});
