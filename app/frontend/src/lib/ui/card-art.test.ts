import { describe, expect, it } from 'vitest';
import { cardTemplate, pickFrom } from './card-art.ts';

describe('wide card template', () => {
	it('never uses titled art: the own backdrop with the logo over it', () => {
		const t = cardTemplate({ art: 'titled.jpg', logo: 'l.png', hasTitle: true }, { backdrop: 'b.jpg', logo: 'k.png' });
		expect(t).toEqual({ variant: 'logo', image: 'b.jpg', logo: 'l.png' });
	});
	it('textless art plus the server logo, else the known logo', () => {
		expect(cardTemplate({ art: 'a.jpg', logo: 'l.png' }, {})).toEqual({ variant: 'logo', image: 'a.jpg', logo: 'l.png' });
		expect(cardTemplate(undefined, { backdrop: 'b.jpg', logo: 'k.png' })).toEqual({ variant: 'logo', image: 'b.jpg', logo: 'k.png' });
	});
	it('the name when there is no logo', () => {
		expect(cardTemplate({ art: 'a.jpg' }, {}).variant).toBe('name');
		expect(cardTemplate(undefined, { backdrop: 'b.jpg' })).toEqual({ variant: 'name', image: 'b.jpg', logo: null });
		expect(cardTemplate(undefined, {})).toEqual({ variant: 'name', image: null, logo: null });
	});
	it('falls back a step when a picture fails', () => {
		expect(cardTemplate({ art: 't.jpg', hasTitle: true }, { backdrop: 'b.jpg', logo: 'k.png' }, { art: true })).toEqual({ variant: 'logo', image: null, logo: 'k.png' });
		expect(cardTemplate({ art: 'a.jpg', logo: 'l.png' }, {}, { logo: true })).toEqual({ variant: 'name', image: 'a.jpg', logo: null });
	});
	it('reads the server fields', () => {
		expect(pickFrom({ card_art: 'a', card_logo: '', art_has_title: true })).toEqual({ art: 'a', logo: null, hasTitle: true });
		expect(pickFrom({})).toBeUndefined();
	});
});
