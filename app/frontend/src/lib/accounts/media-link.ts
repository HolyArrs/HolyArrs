// The owner's account picker in Settings > Users: an account linked to someone else shows whose it is and cannot be picked.
export type ServerAccount = { id: string; name: string; taken_by?: number; taken_name?: string };

export function accountChoice(a: ServerAccount, userId: number, t: (key: string, vars?: Record<string, string | number>) => string): { label: string; taken: boolean } {
	if (!a.taken_by || a.taken_by === userId) return { label: a.name, taken: false };
	return { label: `${a.name} · ${t('settings.users.accountLinked', { name: a.taken_name ?? '' })}`, taken: true };
}
