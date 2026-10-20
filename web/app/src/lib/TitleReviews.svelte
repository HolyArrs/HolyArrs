<script lang="ts">
	import { api } from './api.ts';
	import { date } from './format.ts';
	import { ordersOf, orderWord, pillState, reviewsPath, scoreText, spread, votesText, type Review, type ReviewsAnswer } from './title-reviews.ts';

	// Ratings & reviews: a pill per source the server has set up (TMDB
	// first), the source's score, vote count and spread, then its written
	// reviews as plain text, newest first, a page at a time. Reviews are
	// other people's words: never rendered as HTML.
	let { kind, tmdb }: { kind: 'movie' | 'show'; tmdb: string | null | undefined } = $props();

	let picked = $state('tmdb');
	let order = $state('newest');
	let answer = $state<ReviewsAnswer | null>(null);
	let reviews = $state<Review[]>([]);
	let loading = $state(false);
	let failed = $state(false);
	let open = $state<Record<number, boolean>>({});
	// A few reviews first; More shows the next ones (and reads the next page when the shown ones run out).
	const FIRST = 5;
	let shown = $state(FIRST);
	function more() {
		if (shown >= reviews.length && answer && answer.page < answer.pages) load(answer.page + 1);
		shown += 10;
	}

	const ps = $derived(pillState(answer?.sources, picked));
	const orders = $derived(ordersOf(answer?.sources, ps.active));
	const bars = $derived(spread(answer?.distribution));
	const sourceName = $derived(ps.pills.find((p) => p.id === ps.active)?.name ?? 'TMDB');

	function load(page: number, ctl?: AbortController) {
		const t = tmdb;
		if (!t) return;
		loading = true;
		failed = false;
		api.get<ReviewsAnswer>(reviewsPath(kind, t, picked, page, order), ctl?.signal)
			.then((a) => {
				answer = a;
				reviews = page > 1 ? [...reviews, ...(a.reviews ?? [])] : (a.reviews ?? []);
				if (a.source && a.source !== picked) picked = a.source;
			})
			.catch(() => {
				if (!ctl?.signal.aborted) failed = true;
			})
			.finally(() => (loading = false));
	}

	$effect(() => {
		void kind;
		void tmdb;
		void picked;
		void order;
		const ctl = new AbortController();
		open = {};
		shown = FIRST;
		load(1, ctl);
		return () => ctl.abort();
	});

	function choose(id: string) {
		if (id === picked) return;
		answer = answer && { ...answer, reviews: [] };
		reviews = [];
		order = 'newest';
		picked = id;
	}

	// Read more shows only when the text runs well past its clamp; a review
	// that would hide a line or two is shown whole instead.
	const SLACK = 52;
	function clamped(node: HTMLElement, i: number) {
		const check = () => {
			const li = node.parentElement;
			if (!li || li.hasAttribute('data-fit')) return;
			const extra = node.scrollHeight - node.clientHeight;
			if (extra > 2 && extra <= SLACK && !open[i]) return void li.setAttribute('data-fit', '');
			li.toggleAttribute('data-long', extra > SLACK || !!open[i]);
		};
		queueMicrotask(check);
		const ro = new ResizeObserver(check);
		ro.observe(node);
		return { destroy: () => ro.disconnect() };
	}
</script>

{#if answer && (answer.score > 0 || reviews.length || ps.pills.length > 1)}
	<section class="tp-section rv" aria-labelledby="title-reviews-h" id="title-reviews">
		<div class="tp-head">
			<h2 class="tp-h2" id="title-reviews-h">Ratings &amp; reviews</h2>
			{#if ps.pills.length === 1}
				<span class="pill on" data-testid="review-source">{ps.pills[0].name}</span>
			{:else if ps.pills.length > 1}
				<div class="pills" role="group" aria-label="Review source">
					{#each ps.pills as p (p.id)}
						<button type="button" class="pill" aria-pressed={p.id === ps.active} onclick={() => choose(p.id)}>{p.name}</button>
					{/each}
				</div>
			{/if}
		</div>

		<div class="score-row">
			{#if answer.score > 0}
				<p class="score"><span class="star" aria-hidden="true">★</span> <b>{scoreText(answer.score)}</b><span class="of"> / 10</span></p>
				<p class="muted votes">{votesText(answer.votes)} on {sourceName}</p>
			{:else}
				<p class="muted">No score on {sourceName} yet.</p>
			{/if}
			{#if typeof answer.watching === 'number'}
				<p class="watching"><span aria-hidden="true">👥</span> {answer.watching.toLocaleString()} watching now</p>
			{/if}
		</div>

		{#if bars.length}
			<ol class="bars" aria-label="How people scored it">
				{#each bars as b (b.point)}
					<li title="{b.count.toLocaleString()} scored {b.point}"><span class="pt">{b.point}</span><span class="track"><span class="fill" style="width: {Math.max(b.share * 100, b.count ? 2 : 0)}%"></span></span></li>
				{/each}
			</ol>
		{/if}

		{#if !answer.reviews_hidden}
			{#if orders.length > 1 && reviews.length}
				<div class="pills order" role="group" aria-label="Sort reviews">
					{#each orders as o (o)}
						<button type="button" class="pill" aria-pressed={o === order} onclick={() => (order = o)}>{orderWord(o)}</button>
					{/each}
				</div>
			{/if}
			{#if reviews.length}
				<ul class="list">
					{#each reviews.slice(0, shown) as r, i (i)}
						<li class="review" class:open={open[i]}>
							<div class="who">
								<b>{r.author || 'Someone'}</b>
								{#if r.date}<span class="muted">{date(r.date)}</span>{/if}
								{#if r.rating}<span class="their"><span aria-hidden="true">★</span> {r.rating}<span class="of">/10</span></span>{/if}
								{#if r.likes}<span class="muted">{r.likes.toLocaleString()} likes</span>{/if}
							</div>
							{#if r.spoiler && !open[i]}
								<p class="muted spoiler">Has spoilers.</p>
								<button type="button" class="more" onclick={() => (open[i] = true)}>Show it</button>
							{:else}
								<p class="text" use:clamped={i}>{r.text}</p>
								<button type="button" class="more" onclick={() => (open[i] = !open[i])}>{open[i] ? 'Show less' : 'Read more'}</button>
							{/if}
						</li>
					{/each}
				</ul>
				{#if shown < reviews.length || answer.page < answer.pages}
					<button type="button" class="btn ghost" disabled={loading} onclick={more}>{loading ? 'Loading…' : 'More reviews'}</button>
				{/if}
			{:else if !loading}
				<p class="muted">No written reviews on {sourceName} yet.</p>
			{/if}
		{/if}
		{#if failed}<p class="muted">{sourceName} did not answer; try again shortly.</p>{/if}
		<p class="xs muted credit">{answer.reviews_hidden ? 'Score' : 'Reviews'} from {sourceName}.</p>
	</section>
{/if}

<style>
	.rv { display: flex; flex-direction: column; gap: 14px; }
	.rv .tp-head { margin-bottom: 0; }
	.pills { display: inline-flex; flex-wrap: wrap; gap: 6px; }
	.pill { min-height: 32px; padding: 4px 14px; border: 1px solid var(--line); border-radius: 999px; background: transparent; color: var(--text); font: var(--t-subhead); font-weight: 600; cursor: pointer; }
	.pill[aria-pressed='true'], .pill.on { background: var(--text); color: var(--bg); border-color: var(--text); }
	.score-row { display: flex; flex-wrap: wrap; align-items: baseline; gap: 6px 18px; }
	.score-row p { margin: 0; }
	.score { font: 700 28px/1.1 var(--font-display, var(--font-ui)); letter-spacing: -0.02em; }
	.star { color: #f5c518; }
	.of { color: var(--text-muted); font-weight: 500; font-size: 0.6em; }
	.votes, .watching { font: var(--t-subhead); }
	.bars { display: grid; gap: 3px; max-width: 360px; margin: 0; padding: 0; list-style: none; }
	.bars li { display: grid; grid-template-columns: 18px 1fr; align-items: center; gap: 8px; font: var(--t-footnote); color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.track { height: 6px; border-radius: 3px; background: var(--surface-2); overflow: hidden; }
	.fill { display: block; height: 100%; border-radius: 3px; background: #f5c518; }
	.list { display: flex; flex-direction: column; margin: 0; padding: 0; list-style: none; }
	.review { padding: 14px 0; min-width: 0; }
	.review + .review { border-top: 1px solid var(--line); }
	.who { display: flex; flex-wrap: wrap; align-items: baseline; gap: 4px 12px; font: var(--t-subhead); }
	.their { font-weight: 600; }
	.their [aria-hidden] { color: #f5c518; }
	.text { margin: 6px 0 0; white-space: pre-line; overflow-wrap: anywhere; font: var(--t-body); display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 6; line-clamp: 6; overflow: hidden; }
	.review.open .text, .review:global([data-fit]) .text { display: block; -webkit-line-clamp: unset; line-clamp: unset; }
	.more { display: none; margin-top: 4px; padding: 0; border: 0; background: none; color: var(--accent-text, var(--text)); font: var(--t-subhead); font-weight: 600; cursor: pointer; }
	.review:global([data-long]) .more, .spoiler + .more { display: inline-block; }
	.spoiler { margin: 6px 0 0; font-style: italic; }
	.credit { margin: 0; }
</style>
