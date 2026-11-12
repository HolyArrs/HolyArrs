// Remembers that a lookup answered "off on this install" (Settings > Metadata),
// so a page shows one calm note instead of errors.
export const sourceOff = $state({ seen: false, notices: 0 });

// Called after the owner picks where details come from, so old notes go away.
export function clearSourceOff() {
	sourceOff.seen = false;
}
