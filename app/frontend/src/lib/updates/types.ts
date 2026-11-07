export interface UpdateInfo {
	on: boolean;
	feed: string;
	checked_at?: string;
	latest?: string;
	newer: boolean;
	link?: string;
	no_release?: boolean;
	problem?: string;
	version: string;
}
