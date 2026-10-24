/** The Music Browse page kinds with their own route (genre pages keep /music/genre). */
export function match(param: string): boolean {
	return param === 'mood' || param === 'season' || param === 'decade' || param === 'activity';
}
