import { describe, expect, it } from 'vitest';
import { valueLabel, unitWord } from './library-stats.ts';

const t = (k: string, v?: Record<string, string | number>) => (v ? `${k}:${v.n}` : k);

describe('library stats labels', () => {
	it('names known values and words the special ones', () => {
		expect(valueLabel('video_codec', 'hevc', t)).toBe('HEVC (H.265)');
		expect(valueLabel('hdr', 'sdr', t)).toBe('SDR');
		expect(valueLabel('container', 'matroska', t)).toBe('MKV');
		expect(valueLabel('resolution', '1080p', t)).toBe('1080p');
		expect(valueLabel('hdr', 'other', t)).toBe('library.stats.value.other');
		expect(valueLabel('resolution', 'unchecked', t)).toBe('library.stats.value.unchecked');
		expect(valueLabel('audio_codec', 'unknown', t)).toBe('library.stats.value.unknown');
	});
	it('picks one or other', () => {
		expect(unitWord('movie', 1, t)).toBe('library.stats.unit.movie.one:1');
		expect(unitWord('play', 3, t)).toBe('library.stats.unit.play.other:3');
	});
});
