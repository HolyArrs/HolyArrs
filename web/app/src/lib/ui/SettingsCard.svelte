<script lang="ts" module>
	// Which Advanced disclosures a person left open, per person and per card, in this browser.
	const PREFIX = 'holyarrs.settings.advanced.';
	export function advancedKey(person: string, card: string): string {
		return `${PREFIX}${person || 'someone'}.${card}`;
	}
	function readOpen(key: string): boolean {
		try {
			return localStorage.getItem(key) === '1';
		} catch {
			return false;
		}
	}
	function writeOpen(key: string, open: boolean) {
		try {
			if (open) localStorage.setItem(key, '1');
			else localStorage.removeItem(key);
		} catch {
			// Storage can be blocked; the disclosure still works for this visit.
		}
	}
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import Icon, { type IconName } from '../Icon.svelte';
	import { session } from '../session.svelte.ts';

	// A grouped settings block: title, one-line description, content, an
	// optional Advanced disclosure (closed until the person opens it, then
	// remembered) and an optional footer. tone="danger" outlines it in red.
	let {
		title,
		description,
		icon,
		id,
		tone,
		children,
		advanced,
		advancedId,
		footer
	}: {
		title: string;
		description?: string;
		icon?: IconName;
		id?: string;
		tone?: 'danger';
		children: Snippet;
		advanced?: Snippet;
		/** A stable name for this card's Advanced disclosure; defaults to id or title. */
		advancedId?: string;
		footer?: Snippet;
	} = $props();
	const uid = $props.id();
	const storeKey = $derived(advancedKey(session.s.name, advancedId ?? id ?? title));
	let open = $state(false);
	$effect(() => {
		open = readOpen(storeKey);
	});
	function toggled(e: Event) {
		open = (e.currentTarget as HTMLDetailsElement).open;
		writeOpen(storeKey, open);
	}
</script>

<section class="ui-settings" class:danger={tone === 'danger'} {id} aria-labelledby="{uid}-t" tabindex="-1">
	<header>
		{#if icon}<span class="ic"><Icon name={icon} size={18} /></span>{/if}
		<div>
			<h2 id="{uid}-t">{title}</h2>
			{#if description}<p>{description}</p>{/if}
		</div>
	</header>
	<div class="body">{@render children()}</div>
	{#if advanced}
		<details class="advanced" {open} ontoggle={toggled} data-testid="advanced">
			<summary><Icon name="chevronRight" size={15} /><span>Advanced</span></summary>
			<div class="adv-body">{@render advanced()}</div>
		</details>
	{/if}
	{#if footer}<footer>{@render footer()}</footer>{/if}
</section>

<style>
	.ui-settings { border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); overflow: hidden; scroll-margin-top: calc(var(--topbar, 56px) + 16px); }
	.ui-settings:focus { outline: none; }
	.ui-settings:focus-visible { box-shadow: var(--focus-ring); }
	.danger { box-shadow: 0 0 0 1.5px var(--fail); }
	.danger h2 { color: var(--fail-text); }
	.danger .ic { background: rgb(215 0 21 / 10%); color: var(--fail-text); }
	header { display: flex; gap: 12px; align-items: flex-start; padding: 18px 20px 6px; }
	.ic { width: 32px; height: 32px; flex: none; display: grid; place-items: center; border-radius: 8px; background: var(--accent-soft); color: var(--accent-text); }
	h2 { font: var(--t-headline); font-size: 17px; }
	p { margin-top: 2px; color: var(--text-muted); font: var(--t-callout); }
	.body { padding: 10px 20px 18px; display: flex; flex-direction: column; gap: 12px; }
	.advanced { border-top: 1px solid var(--line); }
	summary {
		display: flex;
		align-items: center;
		gap: 6px;
		min-height: 44px;
		padding: 0 20px;
		cursor: pointer;
		list-style: none;
		color: var(--text-muted);
		font: var(--t-callout);
		font-weight: 600;
	}
	summary::-webkit-details-marker { display: none; }
	summary:hover { color: var(--text); }
	summary :global(.icon) { transition: transform var(--dur-fast) var(--ease-out); }
	.advanced[open] summary :global(.icon) { transform: rotate(90deg); }
	.adv-body { padding: 0 20px 18px; display: flex; flex-direction: column; gap: 12px; }
	footer { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: 8px; padding: 12px 20px; background: var(--surface-2); border-top: 1px solid var(--line); }
</style>
