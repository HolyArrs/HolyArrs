// Types and request helpers for reviewing and removing a movie collection.
import { t } from '../core/i18n/i18n.svelte.ts';

export interface MovieCollection {
	id: number;
	source_id: string;
	name: string;
	owned_titles: number;

	title_ids?: number[];
}

export interface CollectionFile {
	id: number;
	version_id: number;
	path: string;
	origin: string;
	state: string;
	protected: boolean;
}

export interface CollectionTitle {
	id: number;
	name: string;
	year?: number;
	state: string;
	version_ids: number[];
	files: CollectionFile[];
	protected: boolean;
}

export interface CollectionDetail extends MovieCollection {
	titles: CollectionTitle[];
}

export interface CollectionChoice {
	delete_files: boolean;
	never_add: boolean;
	acknowledge_protection: boolean;
}

export interface CollectionReviewTitle extends CollectionTitle {
	selected: boolean;
	skip?: string;
}

export interface CollectionRemoveResult {
	title_id: number;
	name: string;
	status: 'removed' | 'skipped' | 'failed';
	error?: string;
}

export interface CollectionReview {
	collection: MovieCollection;
	titles: CollectionReviewTitle[];
	choice: CollectionChoice;
	review_token: string;
	applied: boolean;
	results?: CollectionRemoveResult[];
}

export function collectionStart(): CollectionChoice {
	return { delete_files: false, never_add: false, acknowledge_protection: false };
}

// Ids are sorted so the same selection always makes the same body and review token.
export function collectionBody(titleIds: number[], choice: CollectionChoice, reviewToken = '') {
	return { title_ids: [...titleIds].sort((a, b) => a - b), ...choice, ...(reviewToken ? { review_token: reviewToken } : {}) };
}

// One plain sentence per title for the result list.
export function collectionResultLine(r: CollectionRemoveResult): string {
	if (r.status === 'removed') return t('mediafiles.collections.removed', { name: r.name });
	if (r.status === 'skipped') return t('mediafiles.collections.skipped', { name: r.name, why: r.error ?? t('mediafiles.collections.wasSkipped') });
	return t('mediafiles.collections.failed', { name: r.name, why: r.error ?? t('mediafiles.collections.tryAgain') });
}
