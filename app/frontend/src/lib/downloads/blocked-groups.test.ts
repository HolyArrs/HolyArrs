import { describe, expect, it } from 'vitest';
import { addBlockedGroup, tidyBody, blankBody } from './pick-rules.ts';

describe('addBlockedGroup', () => {
	it.each([
		{ have: undefined, raw: 'FLUX', want: ['FLUX'] },
		{ have: ['flux'], raw: 'FLUX', want: ['flux'] },
		{ have: ['CAKES'], raw: '  SPARKS ', want: ['CAKES', 'SPARKS'] },
		{ have: ['CAKES'], raw: '   ', want: ['CAKES'] },
		{ have: ['ETHEL'], raw: 'ETH', want: ['ETHEL', 'ETH'] }
	])('$raw into $have', ({ have, raw, want }) => {
		expect(addBlockedGroup(have, raw)).toEqual(want);
	});

	it('drops an empty list on save', () => {
		const b = blankBody('movie');
		b.blocked_groups = [];
		expect(tidyBody(b).blocked_groups).toBeUndefined();
	});
});
