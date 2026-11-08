// Shapes of the health routes' answers.
import type { HealthCheck } from '../core/types.ts';

// Linked apps and stages share one shape so they can sit in one list.
export interface StageState {
	key: string;
	kind: 'stage' | 'source' | 'downloader' | 'subtitle_site' | 'media_server' | 'part';
	name: string;
	state: 'off' | 'starting' | 'running' | 'retrying' | 'failed';
	reason: string;
	since: string;
	stuck: boolean;
	restarts: number;
}

export interface HealthStates {
	safe_mode: boolean;
	states: StageState[];
}

export interface HealthChecks {
	checks: HealthCheck[];
	worst: 'ok' | 'warning' | 'error';
		// Only present when downloads sync is on.
	sync?: { id: number; name: string; health: 'degraded' | 'checking'; stale?: boolean; seen_at?: string }[];
}

export interface HealthSettings {
	every_minutes: number;
	flap_guard: number;
	low_space_mb: number;
	update_check_url: string;
	update_check?: boolean;
	update_check_every_days?: number;
	update_check_local?: boolean;
	update_prerelease?: boolean;
	update_check_default?: string;
}

export interface ActiveJob {
	id: number;
	kind: string;
	label: string;
	state: 'running' | 'queued';
		// Stays 0 until the job reports how far it is.
	progress: number;
	note?: string;
	queued_at: string;
	started_at?: string;
	by_person: boolean;
}

export interface Health {
	ok: boolean;
	version: string;
	schema: number;
	uptime_s: number;
	write_wait_p95_ms: number;
	write_wait_samples: number;
	writes: number;
	sources: { name: string; state: string }[] | null;
	downloaders: { name: string; state: string }[] | null;
	live_clients: number;
	url_base?: string;
}
