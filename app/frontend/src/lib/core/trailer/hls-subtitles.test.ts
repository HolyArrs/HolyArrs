import { afterEach, describe, expect, it, vi } from 'vitest';
import FakeHls from './hls-fake.ts';

vi.mock('../paths.ts', () => ({ to: () => new URL('./hls-fake.ts', import.meta.url).href }));

const { attachHLS } = await import('./hls-player.ts');

vi.stubGlobal('document', { addEventListener() {}, removeEventListener() {}, fullscreenElement: null });
vi.stubGlobal('window', { addEventListener() {}, removeEventListener() {} });

afterEach(() => (FakeHls.made.length = 0));

type Cue = { id: string };
// A text track as the browser keeps it: its cues cannot be read while it is disabled.
class FakeTrack {
	mode: TextTrackMode = 'disabled';
	list: Cue[] = [];
	constructor(
		public kind: string,
		public label: string,
		public language: string
	) {}
	get cues() {
		if (this.mode === 'disabled') return null;
		return Object.assign([...this.list], { getCueById: (id: string) => this.list.find((c) => c.id === id) ?? null });
	}
	addCue(c: Cue) {
		this.list.push(c);
	}
	removeCue(c: Cue) {
		this.list = this.list.filter((x) => x !== c);
	}
}
const video = () => {
	const tracks: FakeTrack[] = [];
	const v = {
		clientWidth: 1280,
		clientHeight: 720,
		dataset: {},
		dispatchEvent() {},
		tracks,
		addTextTrack(kind: string, label: string, language: string) {
			tracks.push(new FakeTrack(kind, label, language));
			return tracks[tracks.length - 1];
		}
	};
	return v as unknown as HTMLVideoElement & { tracks: FakeTrack[] };
};
const url = '/api/v1/trailer/stream/x/manifest.m3u8';
const cues = (...ids: string[]) => ({ type: 'subtitles', track: 'default', cues: ids.map((id) => ({ id })), subtitleTrack: { name: 'English', lang: 'en', default: true } });

describe('attachHLS subtitles', () => {
	// hls.js gives every text track it makes a <track src="data:,WEBVTT">, which
	// the page's policy (media-src) blocks and the browser logs as an error.
	it('keeps hls.js from making track elements', async () => {
		const stop = await attachHLS(video(), url, false);
		const c = FakeHls.made[0].config;
		expect(c.renderTextTracksNatively).toBe(false);
		for (const k of ['enableCEA708Captions', 'enableID3MetadataCues', 'enableEmsgMetadataCues', 'enableDateRangeMetadataCues']) expect(c[k], k).toBe(false);
		expect(c.enableWebVTT).not.toBe(false);
		stop();
	});
	it('shows the cues of the stream’s default subtitle track on a track of its own', async () => {
		const v = video();
		const stop = await attachHLS(v, url, false);
		expect(v.tracks.length).toBe(0); // a stream with no subtitles leaves the video alone
		const hls = FakeHls.made[0];
		hls.emit(FakeHls.Events.CUES_PARSED, cues('a', 'b'));
		hls.emit(FakeHls.Events.CUES_PARSED, cues('b', 'c')); // a part read twice adds nothing twice
		expect(v.tracks.length).toBe(1);
		expect(v.tracks[0]).toMatchObject({ kind: 'subtitles', label: 'English', language: 'en', mode: 'showing' });
		expect(v.tracks[0].list.map((c) => c.id)).toEqual(['a', 'b', 'c']);
		stop();
	});
	it('ignores closed-caption cues', async () => {
		const v = video();
		const stop = await attachHLS(v, url, false);
		FakeHls.made[0].emit(FakeHls.Events.CUES_PARSED, { ...cues('a'), type: 'captions' });
		expect(v.tracks.length).toBe(0);
		stop();
	});
	it('clears them when the trailer stops, and the next trailer on the same video reuses the track', async () => {
		const v = video();
		const stop = await attachHLS(v, url, false);
		FakeHls.made[0].emit(FakeHls.Events.CUES_PARSED, cues('a'));
		stop();
		expect(v.tracks[0].list).toEqual([]);
		expect(v.tracks[0].mode).toBe('disabled');
		const stop2 = await attachHLS(v, url, false);
		FakeHls.made[1].emit(FakeHls.Events.CUES_PARSED, cues('z'));
		expect(v.tracks.length).toBe(1);
		expect(v.tracks[0].list.map((c) => c.id)).toEqual(['z']);
		expect(v.tracks[0].mode).toBe('showing');
		stop2();
	});
	it('drops the old cues when the subtitle track changes', async () => {
		const v = video();
		const stop = await attachHLS(v, url, false);
		const hls = FakeHls.made[0];
		hls.emit(FakeHls.Events.CUES_PARSED, cues('a'));
		hls.emit(FakeHls.Events.SUBTITLE_TRACK_SWITCH, { id: 1 });
		expect(v.tracks[0].list).toEqual([]);
		stop();
	});
});
