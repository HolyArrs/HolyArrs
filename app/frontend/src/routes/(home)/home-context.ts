// What Home's parts read from the page: its answers, flags and a few results the parts hand to each other.
import type { Ask, DownloadRow, LibraryFolder } from '#lib/core/types.ts';
import type { Health, HealthChecks } from '#lib/health/index.ts';
import type { Attention, HomeAnswer, HomeRow, LibraryCardView } from './home.ts';

export interface HomeContext {
	home: HomeAnswer | null;
	folders: LibraryFolder[];
	downloads: DownloadRow[];
	asks: Ask[];
	health: Health | null;
	checks: HealthChecks | null;
	now: Date;
	settled: boolean;
	uncheckedDl: boolean;
	uncheckedHealth: boolean;
	uncheckedParts: boolean;
	uncheckedFolders: boolean;
	admin: boolean;
	manager: boolean;
	owner: boolean;
	kid: boolean;
	canAsk: boolean;
	rows: HomeRow[];
	libCards: LibraryCardView[];
	showDisk: boolean;
	waiting: Ask[];
	fetchingAll: DownloadRow[];
	diskTotal: number;
	worst: string;
	fixes: Attention[];
	unchecked: boolean;
	dlCount: { waiting: number; down: number; imp: number; paused: number };
	speed: number;
	stageWords: (c: { waiting: number; down: number; imp: number; paused: number }) => string;
	askPic: (a: Ask) => string | undefined;
}
