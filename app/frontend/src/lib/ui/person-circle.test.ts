import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import PersonCircle from './PersonCircle.svelte';

describe('PersonCircle', () => {
	it('is a link named for the person and role when it has an href', () => {
		const { body } = render(PersonCircle, { props: { name: 'Harold Perrineau', role: 'Boyd Stevens', href: '/people/6195' } });
		expect(body).toMatch(/<a[^>]*href="\/people\/6195"/);
		expect(body).toContain('aria-label="Harold Perrineau, Boyd Stevens"');
	});
	it('stays a plain block without an href', () => {
		const { body } = render(PersonCircle, { props: { name: 'Pia Novak', role: 'Writer' } });
		expect(body).not.toContain('<a');
		expect(body).not.toContain('aria-label');
	});
});
