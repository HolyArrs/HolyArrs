// The ways to support HolyArrs, shown on Status > Support and in the Why HolyArrs note.
import type { IconName } from '#lib/ui/Icon.svelte';

export type SupportLink = { id: 'sponsor' | 'coffee' | 'star' | 'note'; href: string; icon: IconName; newTab: boolean };

export const supportLinks: SupportLink[] = [
	{ id: 'sponsor', href: 'https://github.com/sponsors/CoderLuii', icon: 'heart', newTab: true },
	{ id: 'coffee', href: 'https://buymeacoffee.com/coderluii', icon: 'coffee', newTab: true },
	{ id: 'star', href: 'https://github.com/HolyArrs/HolyArrs', icon: 'star', newTab: true },
	{ id: 'note', href: 'mailto:support@holyarrs.com?subject=A%20note%20about%20HolyArrs', icon: 'mail', newTab: false },
];
