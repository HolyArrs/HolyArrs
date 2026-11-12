// Stands in for hls.js in tests: its level setters work as in hls.js 1.7, where
// nextLevel and currentLevel pick a fixed level and turn quality switching off.
type Handler = (e: string, d?: unknown) => void;

export default class FakeHls {
	static Events = { MANIFEST_PARSED: 'hlsManifestParsed', AUDIO_TRACKS_UPDATED: 'hlsAudioTracksUpdated', ERROR: 'hlsError' };
	static isSupported() {
		return true;
	}
	static made: FakeHls[] = [];
	levels = [
		{ height: 360, videoCodec: 'avc1.4D401E' },
		{ height: 720, videoCodec: 'avc1.4D401F' },
		{ height: 1080, videoCodec: 'avc1.640028' }
	];
	audioTracks: { name: string; default: boolean }[] = [];
	audioTrack = -1;
	autoLevelCapping = -1;
	startLevel = -1;
	manualLevel = -1;
	handlers = new Map<string, Handler[]>();
	constructor(public config: Record<string, unknown>) {
		FakeHls.made.push(this);
	}
	get autoLevelEnabled() {
		return this.manualLevel === -1;
	}
	get nextLevel() {
		return this.manualLevel;
	}
	set nextLevel(n: number) {
		this.manualLevel = n;
	}
	get currentLevel() {
		return this.manualLevel;
	}
	set currentLevel(n: number) {
		this.manualLevel = n;
	}
	on(e: string, fn: Handler) {
		this.handlers.set(e, [...(this.handlers.get(e) ?? []), fn]);
	}
	emit(e: string, d?: unknown) {
		for (const fn of this.handlers.get(e) ?? []) fn(e, d);
	}
	removeLevel(i: number) {
		this.levels.splice(i, 1);
	}
	loadSource() {}
	attachMedia() {}
	destroy() {}
}
