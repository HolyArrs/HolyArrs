<script lang="ts">
	import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
	// Review shelf for a title: score summary, source pills, sort pills and review cards.
	import Shelf from './Shelf.svelte';
	import Sheet from './Sheet.svelte';
	import { ordersOf, orderWord, pillState, reviewsPath, scoreText, votesText, type Review, type ReviewsAnswer } from './title-reviews.ts';
	import { api } from '../core/api.ts';
	import { date } from '../core/format.ts';
	import { shownScore } from '../core/rating.ts';

	let { kind, tmdb }: { kind: 'movie' | 'show'; tmdb: string | null | undefined } = $props();

	let picked = $state('tmdb');
	let order = $state('newest');
	let answer = $state<ReviewsAnswer | null>(null);
	let reviews = $state<Review[]>([]);
	let loading = $state(false);
	let failed = $state(false);
	let reading = $state<Review | null>(null);
	let sheet = $state(false);
	let seenContent = $state(false);

	const hasContent = $derived(!!answer && reviews.length > 0);
	const ps = $derived(pillState(answer?.sources, picked));
	const orders = $derived(ordersOf(answer?.sources, ps.active));
	const moreToRead = $derived(!!answer && answer.page < answer.pages);

	// Resets before render when the title changes so old content does not keep the shelf open.
	$effect.pre(() => {
		void tmdb;
		seenContent = false;
	});
	// Remembers that content existed, so an emptied source still shows its pills.
	$effect(() => {
		if (hasContent) seenContent = true;
	});
	// Reloads page one on any input change and cancels the request still in flight.
	$effect(() => {
		void kind;
		void tmdb;
		void picked;
		void order;
		const ctl = new AbortController();
		load(1, ctl);
		return () => ctl.abort();
	});

	function load(page: number, ctl?: AbortController) {
		const t = tmdb;
		if (!t) return;
		loading = true;
		failed = false;
		api.get<ReviewsAnswer>(reviewsPath(kind, t, picked, page, order), ctl?.signal)
			.then((a) => {
				answer = a;
				reviews = page > 1 ? [...reviews, ...(a.reviews ?? [])] : (a.reviews ?? []);
				// The server may switch source when the asked one has nothing.
				if (a.source && a.source !== picked) picked = a.source;
			})
			.catch(() => {
				if (!ctl?.signal.aborted) failed = true;
			})
			.finally(() => (loading = false));
	}
	function choose(id: string) {
		if (id === picked) return;
		answer = answer && { ...answer, reviews: [] };
		reviews = [];
		order = 'newest';
		picked = id;
	}
	function read(r: Review) {
		reading = r;
		sheet = true;
	}

	const initial = (name: string) => (name.trim()[0] ?? '?').toUpperCase();
	// Same name always gives the same avatar colour.
	const hue = (name: string) => [...name].reduce((h, c) => (h * 31 + c.charCodeAt(0)) % 360, 7);
	// Matches the card clamp of 5 lines, so only cut-off text gets a More button.
	const long = (r: Review) => r.text.length > 260 || r.text.split('\n').length > 5;
</script>

{#snippet who(r: Review)}
	<div class="who">
		<span class="av" style="--h: {hue(r.author || t('ui.titleReviews.someone'))}" aria-hidden="true">{initial(r.author || t('ui.titleReviews.someone'))}</span>
		<span class="id"><b>{r.author || t('ui.titleReviews.someone')}</b>{#if r.date}<span class="muted">{date(r.date)}</span>{/if}</span>
		{#if r.rating}<span class="their" title={t('ui.titleReviews.theirScoreOutOf')}><span aria-hidden="true">★</span> {r.rating}</span>{/if}
	</div>
{/snippet}

{#if answer && (hasContent || (seenContent && ps.pills.length > 1))}
	<div class="rv" data-testid="title-reviews">
		<Shelf title={t('ui.titleReviews.ratingsReviews')} anchor="title-reviews" item="340px" whole>
			{#snippet tools()}
				<div class="rbar">
					<p class="sum" data-testid="review-score">
						{#if answer && shownScore(answer.score, answer.votes) !== undefined}
							<span class="star" aria-hidden="true">★</span><b>{scoreText(answer.score)}</b><span class="of">/10</span>
							<span class="muted">{votesText(answer.votes)}{typeof answer.watching === 'number' ? ` · ${t('ui.titleReviews.watchingNow', { count: answer.watching.toLocaleString(appLocale()) })}` : ''}</span>
						{:else}
							<span class="muted">{hasContent ? t('ui.titleReviews.noScoreYet') : t('ui.titleReviews.nothingHereYet')}</span>
						{/if}
					</p>
					{#if ps.pills.length > 1}
						<div class="pills" role="group" aria-label={t('ui.titleReviews.reviewSource')}>
							{#each ps.pills as p (p.id)}
								<button type="button" class="pill" aria-pressed={p.id === ps.active} onclick={() => choose(p.id)}>{p.name}</button>
							{/each}
						</div>
					{/if}
					{#if !answer?.reviews_hidden && orders.length > 1 && reviews.length}
						<div class="pills" role="group" aria-label={t('ui.titleReviews.sortReviews')}>
							{#each orders as o (o)}
								<button type="button" class="pill" aria-pressed={o === order} onclick={() => (order = o)}>{orderWord(o)}</button>
							{/each}
						</div>
					{/if}
				</div>
			{/snippet}
			{#if !answer?.reviews_hidden}
				{#each reviews as r, i (i)}
					<article class="rcard review">
						{@render who(r)}
						{#if r.spoiler}
							<p class="text spoiler" aria-label={t('ui.titleReviews.hasSpoilers')}>{r.text}</p>
							<p class="muted warn">{t('ui.titleReviews.hasSpoilers2')}</p>
						{:else}
							<p class="text">{r.text}</p>
						{/if}
						<div class="foot">
							{#if r.likes}<span class="muted">{t('ui.titleReviews.likes', { v: r.likes.toLocaleString(appLocale()) })}</span>{/if}
							{#if r.spoiler || long(r)}<button type="button" class="more" aria-haspopup="dialog" onclick={() => read(r)}>{r.spoiler ? t('ui.titleReviews.showIt') : t('ui.titleReviews.more')}<span class="sr-only"> {t('ui.titleReviews.fromAuthor', { author: r.author || t('ui.titleReviews.someone') })}</span></button>{/if}
						</div>
					</article>
				{/each}
				{#if moreToRead}
					<button type="button" class="rcard next" disabled={loading} aria-busy={loading} onclick={() => answer && load(answer.page + 1)}>
						<span class="plus" aria-hidden="true">+</span>{t('ui.titleReviews.moreReviews')}
					</button>
				{/if}
			{/if}
		</Shelf>
		{#if seenContent && !hasContent && !loading}<p class="muted">{t('ui.titleReviews.noWrittenReviewsHere')}</p>{/if}
		{#if failed}<p class="muted">{t('ui.titleReviews.reviewsCouldNotBe')}</p>{/if}
	</div>
	<Sheet bind:open={sheet} title={reading ? t('ui.titleReviews.reviewBy', { author: reading.author || t('ui.titleReviews.someone') }) : t('ui.titleReviews.review')} size="md">
		{#if reading}
			<div class="full">
				{@render who(reading)}
				<p class="whole">{reading.text}</p>
			</div>
		{/if}
	</Sheet>
{/if}

<style>
	.rv { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
	.rbar { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 14px; }
	.sum { display: inline-flex; flex-wrap: wrap; align-items: baseline; gap: 4px 6px; margin: 0 6px 0 0; font: var(--t-subhead); }
	.sum b { font: 700 20px/1 var(--font-ui); letter-spacing: -0.01em; }
	.sum .star { color: #f5c518; font-size: 16px; }
	.sum .muted { margin-left: 4px; }
	.sum .of { font-size: 13px; }
	.pills { display: inline-flex; flex-wrap: wrap; gap: 6px; }
	.pill { min-height: 32px; padding: 4px 14px; border: 1px solid var(--line); border-radius: 999px; background: transparent; color: var(--text); font: var(--t-subhead); font-weight: 600; cursor: pointer; }
	.pill[aria-pressed='true'] { background: var(--text); color: var(--bg); border-color: var(--text); }
	.rcard { position: relative; display: flex; flex-direction: column; gap: 10px; height: 236px; padding: 16px 18px; border-radius: var(--r-card); background: var(--surface-1); border: 1px solid var(--line); min-width: 0; transition: transform var(--dur) var(--ease-spring), box-shadow var(--dur) var(--ease-out); }
	.review:hover { transform: translateY(-2px); box-shadow: var(--shadow-lift); }
	.rcard p { margin: 0; }
	.star, .their [aria-hidden] { color: #f5c518; }
	.of { color: var(--text-muted); font-weight: 500; font-size: 0.55em; }
	.who { display: flex; align-items: center; gap: 10px; min-width: 0; }
	.av { flex: none; display: grid; place-items: center; width: 34px; height: 34px; border-radius: 50%; background: hsl(var(--h) 45% 38%); color: #fff; font: 700 14px/1 var(--font-ui); }
	.id { display: flex; flex-direction: column; gap: 2px; min-width: 0; font: var(--t-subhead); }
	.id b { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.id .muted { font: var(--t-footnote); }
	.their { margin-left: auto; flex: none; padding: 4px 9px; border-radius: 999px; background: var(--surface-2); font: 700 12px/1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.text { white-space: pre-line; overflow-wrap: anywhere; font: 400 14.5px/1.45 var(--font-ui); display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 5; line-clamp: 5; overflow: hidden; }
	.text.spoiler { filter: blur(6px); user-select: none; -webkit-line-clamp: 3; line-clamp: 3; flex: none; }
	.warn { flex: 1; font: italic var(--t-subhead); }
	.foot { margin-top: auto; display: flex; align-items: center; gap: 10px; min-height: 28px; font: var(--t-footnote); }
	.more { margin-left: auto; min-height: 32px; padding: 0 14px; border: 0; border-radius: 999px; background: var(--surface-2); color: var(--text); font: 600 13px/1 var(--font-ui); cursor: pointer; }
	.more:hover { background: var(--surface-3, var(--surface-2)); }
	.next { align-items: center; justify-content: center; gap: 10px; color: var(--text); font: 600 15px/1 var(--font-ui); cursor: pointer; border-style: dashed; background: transparent; }
	.next:hover { background: var(--surface-1); }
	.plus { display: grid; place-items: center; width: 44px; height: 44px; border-radius: 50%; background: var(--surface-2); font: 400 26px/1 var(--font-ui); }
	.full { display: flex; flex-direction: column; gap: 14px; }
	.whole { margin: 0; white-space: pre-line; overflow-wrap: anywhere; font: var(--t-body); }
	.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
	@media (prefers-reduced-motion: reduce) { .rcard { transition: none; } }
</style>
