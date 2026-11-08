import { describe, expect, it } from 'vitest';
import { linkLabel, runTone, runWord, splitStates } from './checks.ts';
import type { StageState } from './types.ts';

const st = (over: Partial<StageState>): StageState => ({
	key: 'stage:x',
	kind: 'stage',
	name: 'X',
	state: 'running',
	reason: '',
	since: '',
	stuck: false,
	restarts: 0,
	...over
});

describe('stage states', () => {
	it('says each of the five states, and Stuck', () => {
		expect(['off', 'starting', 'running', 'retrying', 'failed'].map((s) => runWord(st({ state: s as StageState['state'] })))).toEqual([
			'Off',
			'Starting',
			'Running',
			'Retrying',
			'Failed'
		]);
		expect(runWord(st({ state: 'failed', stuck: true }))).toBe('Stuck');
	});
	it('colours running green, failed red, off plain and the rest waiting', () => {
		expect([runTone('running'), runTone('failed'), runTone('off'), runTone('retrying'), runTone('starting')]).toEqual(['ok', 'fail', '', 'wait', 'wait']);
	});
	it('names linked app kinds', () => {
		expect(linkLabel('subtitle_site')).toBe('Subtitle site');
		expect(linkLabel('media_server')).toBe('Media server');
		expect(linkLabel('other')).toBe('other');
	});
	it('splits stages from linked apps', () => {
		const s = splitStates([st({}), st({ key: 'link:source:1', kind: 'source' })]);
		expect(s.stages.length).toBe(1);
		expect(s.links[0].kind).toBe('source');
		expect(splitStates(null)).toEqual({ stages: [], links: [] });
		expect(splitStates([st({ key: 'part:music', kind: 'part' })])).toEqual({ stages: [], links: [] });
	});
});
