// Shapes for the default settings applied to new media titles.

// Starting choices for one kind of title.
export interface KindDefaults {
	library_folder_id: number;
	pick_rules_id: number;
	keep_up_to_date: boolean;
	search_on_add: boolean;
}

// Each kind keeps its own defaults.
export interface TitleDefaults {
	movie: KindDefaults;
	show: KindDefaults;
}

// Controls how long a release waits before it can be searched.
export interface GateDefaults {
	release_gate: string;
	gate_delay_days: number;
	bulk_search_respects_gate: boolean;
}
