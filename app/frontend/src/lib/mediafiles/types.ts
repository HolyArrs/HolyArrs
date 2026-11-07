
export interface KindDefaults {
	library_folder_id: number;
	pick_rules_id: number;
	keep_up_to_date: boolean;
	search_on_add: boolean;
}

export interface TitleDefaults {
	movie: KindDefaults;
	show: KindDefaults;
}

export interface GateDefaults {
	release_gate: string;
	gate_delay_days: number;
	bulk_search_respects_gate: boolean;
}
