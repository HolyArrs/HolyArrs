/** A whole number, 0 and up. */
export const wholeNumber = (v: unknown): boolean => /^\d+$/.test(String(v ?? '').trim());

const blank = (v: unknown) => String(v ?? '').trim() === '';

/** Number fields that block Save: a cleared box blocks only when the field has a default to lose. */
export const badNumbers = (fields: { key: string; type: string; default?: string }[], vals: Record<string, unknown>): string[] =>
	fields.filter((f) => f.type === 'number' && !(blank(vals[f.key]) ? blank(f.default) : wholeNumber(vals[f.key]))).map((f) => f.key);
