<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import type { RecentPerson, RecentWay } from './recent-people.ts';
	import SigninFace from './SigninFace.svelte';

	type Props = {
		shown: RecentPerson[];
		editing: boolean;
		wayName: (w: RecentWay) => string;
		dropPerson: (name: string) => void;
		openPerson: (i: number) => void;
		another: () => void;
	};
	let { shown, editing, wayName, dropPerson, openPerson, another }: Props = $props();
</script>

<div class="accts" class:editing role="list" aria-label={t('accounts.signin.recent')} data-testid="recent-people">
	{#each shown as p, i (p.name)}
		<div class="acct" role="listitem">
			<button type="button" class="acct-pick" aria-label={editing ? t('accounts.signin.forgetName', { name: p.name }) : undefined} onclick={() => (editing ? dropPerson(p.name) : openPerson(i))}>
				<SigninFace {p} size={36} />
				<span class="acct-name"><span class="acct-n">{p.name}</span><span class="acct-way">{wayName(p.way)}</span></span>
				<span class="acct-end" aria-hidden="true">{editing ? t('accounts.signin.forget') : '›'}</span>
			</button>
		</div>
	{/each}
	<!-- While editing, every row forgets; the add row steps aside. -->
	{#if !editing}
		<div class="acct" role="listitem">
			<button type="button" class="acct-pick" onclick={another}>
				<SigninFace plus />
				<span class="acct-name"><span class="acct-n">{t('accounts.signin.another')}</span></span>
				<span class="acct-end" aria-hidden="true">›</span>
			</button>
		</div>
	{/if}
</div>

<style>
	:global(.light) .acct-way { color: rgb(0 0 0 / 0.64); }
	:global(.light) .accts { border-color: rgb(0 0 0 / 0.08); }
	:global(.light) .acct + .acct { border-top-color: rgb(0 0 0 / 0.07); }
	/* Rest faint, hover clearly lit: a small step reads as the wrong row when the poster's tint shifts under the glass. */
	:global(.light) .acct-pick { background: rgb(255 255 255 / 0.22); color: #111; }
	@media (hover: hover) { :global(.light) .acct-pick:hover { background: rgb(255 255 255 / 0.8); }
}
	:global(.light) .acct-pick:active { background: rgb(255 255 255 / 0.92); }
	:global(.light) .acct-pick:focus-visible { outline-color: #1d1d1f; }
	:global(.light) .acct-end { color: rgb(0 0 0 / 0.35); }
	:global(.light) .accts.editing .acct-end { color: #1d1d1f; }
	/* People who signed in here: a list of rows, or one person's step. */
	.accts { display: flex; flex-direction: column; border: 1px solid rgb(255 255 255 / 0.1); border-radius: 16px; overflow: hidden; }
	.acct + .acct { border-top: 1px solid rgb(255 255 255 / 0.08); }
	.acct-pick {
		display: flex;
		align-items: center;
		gap: 14px;
		width: 100%;
		min-height: 64px;
		padding: 12px 18px 12px 14px;
		border: 0;
		background: rgb(255 255 255 / 0.03);
		color: #f5f5f7;
		font: 500 15px/1.2 var(--font-ui);
		text-align: left;
		cursor: pointer;
		transition: background-color var(--dur-fast) var(--ease-out);
	}
	/* One ring, inside the row (the global ring would bleed onto the next row). */
	.acct-pick:focus-visible { outline: 2px solid var(--accent-text); outline-offset: -2px; box-shadow: none; }
	@media (hover: hover) {
		.acct-pick:hover { background: rgb(255 255 255 / 0.12); }
		/* The arrow of the row under the pointer steps forward and darkens or brightens. */
		.acct-pick:hover .acct-end { color: currentColor; transform: translateX(2px); }
}
	.acct-pick:active { background: rgb(255 255 255 / 0.16); }
	.acct-end { transition: transform var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out); }
	.acct-name { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
	.acct-n { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.acct-way { font: 400 12px/1.2 var(--font-ui); color: rgb(255 255 255 / 0.55); white-space: nowrap; }
	.acct-end { margin-left: auto; flex: none; color: rgb(255 255 255 / 0.4); font-size: 18px; }
	.accts.editing .acct-end { color: var(--accent-text); font: 500 13px/1 var(--font-ui); }
	/* Phones: the desktop card, floating low over full-bleed art with 24 px of art on every side; it rides above the keyboard (--kb) with a smooth lift. */
	@media (max-width: 600px) {
		.acct-pick { min-height: 60px; }
}
</style>
