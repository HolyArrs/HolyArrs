<script lang="ts">
	import { to } from '#lib/paths.ts';
	import Icon from '#lib/Icon.svelte';
	import { sections as all, visible } from '#lib/settings.ts';
	import { session } from '#lib/session.svelte.ts';
	import { partsNow } from '#lib/parts.svelte.ts';
	import { offPartName } from '#lib/parts.ts';
	// Pages of a part that is off are hidden here too.
	const sections = $derived(visible(all, session.can).filter((s) => !offPartName(partsNow.answer, s.href)));
	import { theme, setTheme } from '#lib/theme.svelte.ts';
</script>

<svelte:head><title>Settings · HolyArrs</title></svelte:head>

<header class="page-head">
	<div>
		<h1>Settings</h1>
		<p>Everything HolyArrs needs to run, one page each.</p>
	</div>
</header>

<ul class="grid">
	{#each sections as s, i (s.href)}
		<li class="rise" style="--i: {i}">
			<a class="tile" href={to(s.href)}>
				<span class="ic"><Icon name={s.icon} /></span>
				<span class="grow">
					<span class="t">{s.label}</span>
					<span class="d xs muted">{s.about}</span>
				</span>
				<Icon name="arrow" size={16} />
			</a>
		</li>
	{/each}
</ul>

<section class="card appearance" aria-labelledby="h-look">
	<h2 id="h-look" class="h2">Appearance</h2>
	<div class="row" role="radiogroup" aria-label="Theme" style="margin-top: var(--s-3)">
		<label class="checkline"><input type="radio" name="theme" checked={theme.value === 'dark'} onchange={() => setTheme('dark')} /> Dark</label>
		<label class="checkline"><input type="radio" name="theme" checked={theme.value === 'light'} onchange={() => setTheme('light')} /> Light</label>
	</div>
	<p class="hint">Saved in this browser. Movement follows your system's reduce-motion setting.</p>
</section>

<style>
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
		gap: var(--s-3);
	}
	.tile {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		padding: var(--s-4);
		min-height: 76px;
		background: var(--surface-1);
		border: 1px solid var(--line);
		border-radius: var(--r-card);
		transition:
			border-color var(--dur),
			transform var(--dur) var(--ease-out);
	}
	.tile:hover {
		border-color: var(--line-strong);
		transform: translateY(var(--lift));
	}
	.tile > :global(.icon:last-child) {
		color: var(--text-faint);
	}
	.ic {
		width: 40px;
		height: 40px;
		display: grid;
		place-items: center;
		border-radius: var(--r-btn);
		background: var(--surface-3);
		color: var(--text);
		flex: none;
	}
	.t {
		display: block;
		font-weight: var(--weight-semibold);
	}
	.d {
		display: block;
	}
	.appearance {
		margin-top: var(--s-6);
	}
</style>
