import { afterEach, describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import LibraryPoster from './LibraryPoster.svelte';
import Avatar from '../ui/Avatar.svelte';
import { setBase } from '../core/paths.ts';

afterEach(() => setBase(''));

const props = (image: string) => ({ title: 'Heat', year: 1995, href: '/library/3', image, status: null, bar: { tone: 'ok' as const, fill: 1 }, kept: true });

describe('posters under a URL base', () => {
	it('asks for a poster view picture with the base in front', () => {
		setBase('/holyarrs');
		const { body } = render(LibraryPoster, { props: props('/api/v1/artwork/6a19a933c1b7afe61a33dea889a53e06.jpg') });
		expect(body).toContain('src="/holyarrs/api/v1/artwork/6a19a933c1b7afe61a33dea889a53e06.jpg"');
		expect(body).not.toContain('src="/api/');
	});
	it('keeps a TMDB poster going through the picture cache with the base', () => {
		setBase('/holyarrs');
		const { body } = render(LibraryPoster, { props: props('https://image.tmdb.org/t/p/w500/a.jpg') });
		expect(body).toContain('src="/holyarrs/api/v1/artwork/tmdb/');
	});
	it('changes nothing without a base', () => {
		const { body } = render(LibraryPoster, { props: props('/api/v1/artwork/6a19a933c1b7afe61a33dea889a53e06.jpg') });
		expect(body).toContain('src="/api/v1/artwork/6a19a933c1b7afe61a33dea889a53e06.jpg"');
	});
	it('shows a person picture with the base in front', () => {
		setBase('/holyarrs');
		const { body } = render(Avatar, { props: { name: 'owner', src: '/api/v1/users/1/avatar?v=abc' } });
		expect(body).toContain('src="/holyarrs/api/v1/users/1/avatar?v=abc"');
	});
});
