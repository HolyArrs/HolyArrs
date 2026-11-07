export interface ApiKey {
	id: number;
	label: string;
	hint: string;
	created_at: string;
	last_used_at?: string;
	/** What the key may reach; ['mcp'] is an AI key. */
	scopes?: string[];
}
