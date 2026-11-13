// The Quality row: the owner's lines of what people can ask for, as buttons on the request sheets and in the approver's request window.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import type { Ask } from '#lib/core/types.ts';
import type { QualityChoice } from './types.ts';

/** The built-in lines. */
export const STANDARD = 'standard';
export const FOUR_K = '4k';

/** One Quality button. `quality` is the line it asks or approves as; `rulesId` other Pick rules on the main copy (approvers only). */
export interface QualityPill {
	key: string;
	label: string;
	quality: string;
	rulesId: number;
	/** It asks for a copy kept apart from the main one (4K). */
	extra: boolean;
	disabled: boolean;
	/** Why it is off ("In library"), shown beside the name. */
	note: string;
}

/** The name people see for a line: the owner's name, else the built-in one. */
export function qualityName(id: string, name?: string): string {
	if (name) return name;
	return id === FOUR_K ? '4K' : t('requests.quality.standard');
}

/** The line an ask was made for (older answers carry only four_k). */
export function askedQuality(a: Pick<Ask, 'quality' | 'four_k'>): string {
	return a.quality || (a.four_k ? FOUR_K : STANDARD);
}

/** The tag a request card shows: the line's name when it is not the first line (Standard); '' otherwise. */
export function qualityTag(a: Pick<Ask, 'kind' | 'quality' | 'quality_name' | 'four_k'>): string {
	if (a.kind === 'artist') return '';
	const id = askedQuality(a);
	return id === STANDARD ? '' : qualityName(id, a.quality_name);
}

export interface MemberRowFacts {
	/** The title is in the Library (its main copy is there). Movies only. */
	inLibrary?: boolean;
	/** The title has its 4K copy. Movies only. */
	has4k?: boolean;
	/** Lines this person already asked for. */
	mine?: string[];
	/** Lines someone else has an open ask for (asking joins it). */
	open?: string[];
	/** The line to start on, when it can be asked (opened from a 4K button). */
	prefer?: string;
}

/**
 * The row on a member's request sheet: one button per line the person may use, in the owner's order. A line whose copy is in the
 * Library, or that the person asked for already, is off. The preferred line, else a line someone else asked for, else the first
 * free one is picked. The row shows only when there is more than one line.
 */
export function memberRow(choices: QualityChoice[], f: MemberRowFacts = {}): { pills: QualityPill[]; selected: string; shown: boolean } {
	const pills = choices.map((c): QualityPill => {
		const have = c.extra ? !!f.has4k : !!f.inLibrary;
		const mine = !have && !!f.mine?.includes(c.id);
		return { key: c.id, label: qualityName(c.id, c.name), quality: c.id, rulesId: 0, extra: c.extra, disabled: have || mine, note: have ? t('mediafiles.state.inLibrary') : mine ? t('discover.titlePage.requestedCaption') : '' };
	});
	const free = (id?: string) => !!id && pills.some((p) => p.key === id && !p.disabled);
	const selected = (free(f.prefer) ? f.prefer : f.open?.find(free)) ?? pills.find((p) => !p.disabled)?.key ?? '';
	return { pills, selected, shown: pills.length > 1 };
}

/**
 * The row in the approver's request window: the main-copy lines, then the approver's other Pick rules (on the main copy), then the
 * lines kept as their own copy (4K). The line that was asked for is there even when it is hidden now, and it is the one picked.
 */
export function ownerRow(
	lines: QualityChoice[],
	rules: { id: number; name: string }[],
	a: Pick<Ask, 'quality' | 'quality_name' | 'four_k'>
): { pills: QualityPill[]; asked: string } {
	const asked = askedQuality(a);
	const all = lines.some((l) => l.id === asked) ? lines : [...lines, { id: asked, name: a.quality_name ?? '', extra: asked === FOUR_K, pick_rules_id: 0 }];
	const pill = (l: QualityChoice): QualityPill => ({ key: l.id, label: qualityName(l.id, l.name), quality: l.id, rulesId: 0, extra: l.extra, disabled: false, note: '' });
	const mains = all.filter((l) => !l.extra);
	const used = new Set(all.map((l) => l.pick_rules_id).filter(Boolean));
	// A rule named like a line (Standard, 4K) is that line to the eye: one button each.
	const names = new Set(all.map((l) => qualityName(l.id, l.name).toLowerCase()));
	const main = mains[0]?.id ?? STANDARD;
	const other = rules.filter((r) => !used.has(r.id) && !names.has(r.name.toLowerCase())).map((r): QualityPill => ({ key: `rules-${r.id}`, label: r.name, quality: main, rulesId: r.id, extra: false, disabled: false, note: '' }));
	return { pills: [...mains.map(pill), ...other, ...all.filter((l) => l.extra).map(pill)], asked };
}
