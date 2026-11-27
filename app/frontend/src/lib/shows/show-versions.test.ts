import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import ShowVersions from './ShowVersions.svelte';
import type { TitleView, VersionView } from '../core/types.ts';

const v = (id: number, main: boolean): VersionView => ({ id, label: main ? 'Main' : '4K', main, pick_rules_id: 1, pick_rules_name: 'Any', keep_up_to_date: true, audio_languages: [], folder_path: '/tv/Show' }) as unknown as VersionView;
const show = (versions: VersionView[]) => ({ id: 1, name: 'Show', versions }) as unknown as TitleView;

describe('a show Versions card (#796)', () => {
	it('shows staff the Change button with only one Version', () => {
		const { body } = render(ShowVersions, { props: { title: show([v(1, true)]), staff: true, onchange: () => {} } });
		expect(body).toContain('data-testid="show-versions"');
		expect(body).toContain('data-testid="version"');
		expect(body).not.toContain('aria-controls="show-versions-list"');
	});
	it('stays hidden for viewers with one Version', () => {
		const { body } = render(ShowVersions, { props: { title: show([v(1, true)]), staff: false, onchange: () => {} } });
		expect(body).not.toContain('data-testid="show-versions"');
	});
	it('keeps See all with two Versions', () => {
		const { body } = render(ShowVersions, { props: { title: show([v(1, true), v(2, false)]), staff: false, onchange: () => {} } });
		expect(body).toContain('aria-controls="show-versions-list"');
	});
});
