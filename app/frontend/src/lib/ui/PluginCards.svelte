<script lang="ts">
	// Cards from plugins (Home widgets, title page sections), drawn by HolyArrs from plain data.
	import { cardLink, type PluginCard } from './plugin-cards.ts';

	let { cards, testid = 'plugin-cards' }: { cards: PluginCard[]; testid?: string } = $props();
</script>

{#if cards.length}
	<div class="cards" data-testid={testid}>
		{#each cards as c (c.plugin)}
			<section class="card" aria-label={c.title}>
				<h2 dir="auto">{c.title}</h2>
				{#if c.rows.length}
					<ul>
						{#each c.rows as r, i (i)}
							{@const href = cardLink(r.link)}
							<li>
								{#if href}<a {href} dir="auto">{r.text}</a>{:else}<span dir="auto">{r.text}</span>{/if}
								{#if r.sub}<span class="sub" dir="auto">{r.sub}</span>{/if}
							</li>
						{/each}
					</ul>
				{/if}
			</section>
		{/each}
	</div>
{/if}

<style>
	.cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 280px), 1fr)); gap: 12px; }
	.card { min-width: 0; padding: 16px; border-radius: 20px; background: linear-gradient(180deg, var(--sheet-a), var(--sheet-b)); box-shadow: var(--edge), var(--drop); }
	h2 { font: var(--t-headline); overflow-wrap: anywhere; }
	ul { list-style: none; margin: 8px 0 0; padding: 0; display: flex; flex-direction: column; gap: 6px; }
	li { display: flex; flex-direction: column; min-width: 0; }
	li a, li span { overflow-wrap: anywhere; }
	li a { color: var(--text); text-decoration: underline; text-underline-offset: 2px; min-height: 24px; }
	li a:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.sub { color: var(--text-muted); font: var(--t-footnote); }
	@media (forced-colors: active) { .card { border: 1px solid CanvasText; } }
</style>
