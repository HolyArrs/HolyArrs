import { defineParams } from '@sveltejs/kit/params';
import { match as musicpage } from './musicpage.ts';

/** Route parameter matchers (SvelteKit 3 reads them from this one entry; each answers the param, or undefined when it does not match). */
export const params = defineParams({
	musicpage: (param: string) => (musicpage(param) ? param : undefined)
});
