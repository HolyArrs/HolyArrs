/** A whole number, 0 and up. */
export const wholeNumber = (v: unknown): boolean => /^\d+$/.test(String(v ?? '').trim());

/**
 * Number fields that block Save: a number setting must hold a whole number, so a box the owner
 * cleared blocks Save with the field's message instead of saving an empty value, which the backend
 * would store as "" and so drop the manifest's default. A number field gets no other check, so an
 * empty box now blocks exactly like letters or a negative number do.
 */
export const badNumbers = (fields: { key: string; type: string; default?: string }[], vals: Record<string, unknown>): string[] =>
	fields.filter((f) => f.type === 'number' && !wholeNumber(vals[f.key])).map((f) => f.key);
