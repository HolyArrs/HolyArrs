// Shapes and labels for the Library stats page.

export type Bucket = { value: string; titles: number; files: number; bytes: number };
export type KindStats = { kind: string; titles: number; files: number; bytes: number; facts: Record<string, Bucket[]> };
export type WatchedPerson = {
	server: string;
	server_kind: string;
	name: string;
	linked: boolean;
	user_id?: number;
	user_name?: string;
	movies: number;
	shows: number;
	episodes: number;
	plays: number;
	last_watched_at?: string;
};
export type Watched = { enabled: boolean; read_at?: string; people: WatchedPerson[] };

type T = (key: string, vars?: Record<string, string | number>) => string;

export const factOrder = ['resolution', 'video_codec', 'hdr', 'audio_codec', 'audio_channels', 'container'];

// Codec and format names are the same in every language.
const names: Record<string, Record<string, string>> = {
	video_codec: { h264: 'AVC (H.264)', hevc: 'HEVC (H.265)', av1: 'AV1', vp9: 'VP9', mpeg2: 'MPEG-2', xvid: 'Xvid', vc1: 'VC-1' },
	hdr: { hdr10: 'HDR10', hlg: 'HLG', dv: 'Dolby Vision', 'dv+hdr10': 'Dolby Vision + HDR10', sdr: 'SDR' },
	container: { matroska: 'MKV', webm: 'WebM', mp4: 'MP4', flac: 'FLAC', mp3: 'MP3', m4a: 'M4A', ogg: 'OGG', opus: 'Opus', wav: 'WAV' },
	audio_codec: { aac: 'AAC', ac3: 'AC3', eac3: 'EAC3', dts: 'DTS', truehd: 'TrueHD', flac: 'FLAC', alac: 'ALAC', mp3: 'MP3', mp2: 'MP2', opus: 'Opus', vorbis: 'Vorbis', pcm: 'PCM', wavpack: 'WavPack', tta: 'TTA' }
};

export function valueLabel(fact: string, value: string, t: T): string {
	if (value === 'other' || value === 'unchecked' || value === 'unknown') return t('library.stats.value.' + value);
	return names[fact]?.[value] ?? value;
}

export function unitWord(unit: 'movie' | 'episode' | 'play', n: number, t: T): string {
	return t(`library.stats.unit.${unit}.${n === 1 ? 'one' : 'other'}`, { n: n.toLocaleString() });
}
