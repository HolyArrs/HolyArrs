// Names of people who signed in with a password on this device, newest first,
// so a shared screen (a TV, a family tablet) can switch between them. Names
// only: each switch is still a full sign-in with the password and any code.
const KEY = 'holyarrs.recent-people';
const MAX = 6;

export function recentPeople(): string[] {
	try {
		const v = JSON.parse(localStorage.getItem(KEY) ?? '[]');
		return Array.isArray(v) ? v.filter((n): n is string => typeof n === 'string' && n !== '').slice(0, MAX) : [];
	} catch {
		return [];
	}
}

function save(names: string[]) {
	try {
		localStorage.setItem(KEY, JSON.stringify(names.slice(0, MAX)));
	} catch {
		// Private windows may refuse storage; the picker just stays empty.
	}
}

export function rememberPerson(name: string) {
	if (!name) return;
	save([name, ...recentPeople().filter((n) => n !== name)]);
}

export function forgetPerson(name: string) {
	save(recentPeople().filter((n) => n !== name));
}
