import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { HLS_VERSION, hlsScript, holdStart } from './hls-player.ts';

describe('hls.js', () => {
	it('is pinned to one exact version, shipped as its own file', () => {
		const pkg = JSON.parse(readFileSync(new URL('../../../../package.json', import.meta.url), 'utf8'));
		expect(pkg.dependencies['hls.js']).toBe(HLS_VERSION);
		expect(hlsScript).toBe(`/vendor/hls-${HLS_VERSION}.min.js`);
	});
});

// A video the guard can watch: its time, state and events, set by the test.
class FakeVideo extends EventTarget {
	currentTime = 0;
	paused = false;
	seeking = false;
	controls = false;
	webkitDisplayingFullscreen = false;
	at(t: number, ev = 'timeupdate') {
		this.currentTime = t;
		this.dispatchEvent(new Event(ev));
	}
}
const video = () => new FakeVideo() as FakeVideo & HTMLVideoElement;

describe('holdStart', () => {
	it('jumps to the start when the picture plays from 0 (a start the browser dropped)', () => {
		const v = video();
		holdStart(v, 146.97);
		v.at(0.4, 'playing');
		expect(v.currentTime).toBe(146.97);
	});
	it('leaves a video that plays from its start, and one less than 2 s early', () => {
		const v = video();
		holdStart(v, 146.97);
		v.at(145.5);
		expect(v.currentTime).toBe(145.5);
		v.at(150);
		expect(v.currentTime).toBe(150);
	});
	it('catches a later drop back to 0 too (a stream recovery)', () => {
		const v = video();
		holdStart(v, 146.97);
		v.at(150);
		v.at(0.2);
		expect(v.currentTime).toBe(146.97);
	});
	it('keeps a seek made in the video player itself (iPhone full screen)', () => {
		const v = video();
		holdStart(v, 146.97);
		v.at(150);
		v.webkitDisplayingFullscreen = true;
		v.at(10, 'seeking');
		v.at(10.5);
		expect(v.currentTime).toBe(10.5);
	});
	it('waits while paused or seeking', () => {
		const v = video();
		holdStart(v, 146.97);
		v.paused = true;
		v.at(0.1);
		v.paused = false;
		v.seeking = true;
		v.at(0.2);
		expect(v.currentTime).toBe(0.2);
	});
	it('gives up after 3 tries at a start it cannot reach', () => {
		const v = video();
		holdStart(v, 146.97);
		const seen: number[] = [];
		for (let i = 0; i < 4; i++) {
			v.at(0.1);
			seen.push(v.currentTime);
		}
		expect(seen).toEqual([146.97, 146.97, 146.97, 0.1]);
	});
	it('stops when let go', () => {
		const v = video();
		holdStart(v, 146.97)();
		v.at(0.3, 'playing');
		expect(v.currentTime).toBe(0.3);
	});
	it('does nothing for a start of 0', () => {
		const v = video();
		holdStart(v, 0);
		v.at(0.5);
		expect(v.currentTime).toBe(0.5);
	});
});

describe('BUFFER', () => {
	it('caps every hls.js limit that lets the buffer grow past about 20 s', async () => {
		const { BUFFER } = await import('./hls-player.ts');
		expect(BUFFER.maxBufferLength).toBe(20);
		expect(BUFFER.maxMaxBufferLength).toBeLessThanOrEqual(30);
		expect(BUFFER.maxBufferSize).toBeLessThanOrEqual(15_000_000);
		expect(BUFFER.backBufferLength).toBeLessThanOrEqual(10);
		expect(BUFFER.startFragPrefetch).toBe(false);
	});
});
