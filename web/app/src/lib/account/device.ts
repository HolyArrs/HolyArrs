import type { IconName } from '../Icon.svelte';

/** deviceIcon is the glyph for a signed-in device (the icon set has one screen glyph). */
export function deviceIcon(device: string): IconName {
	void device;
	return 'tv';
}
