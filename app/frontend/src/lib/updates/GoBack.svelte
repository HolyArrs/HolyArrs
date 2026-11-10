<script lang="ts">
	import Words from '#lib/ui/Words.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import { IMAGE, goBackBackup, imageTag, type BackupRow } from './update-notice.ts';
	import { api } from '../core/api.ts';
	import { dateTime } from '../core/format.ts';
	import { toasts } from '../core/toasts.svelte.ts';

	let { version, canAct }: { version: string | undefined; canAct: boolean } = $props();

	let goBack = $state<BackupRow | null>(null);
	$effect(() => {
		const v = version;
		if (!canAct || !v) return;
		api.get<{ backups: BackupRow[] }>('/backups')
			.then((r) => (goBack = goBackBackup(r.backups, v)))
			.catch(() => (goBack = null));
	});

	async function copy(text: string) {
		try {
			await navigator.clipboard.writeText(text);
			toasts.push(t('updates.goBack.copied'), 'ok');
		} catch {
			toasts.push(t('updates.goBack.copyFailed'), 'fail', { body: t('updates.goBack.copyFailedBody') });
		}
	}
</script>

{#if goBack?.from_version}
	{@const tag = imageTag(goBack.from_version)}
	<section class="st-plate go-back" aria-labelledby="s-goback" data-testid="go-back">
		<h2 id="s-goback">{t('updates.goBack.title', { version: goBack.from_version })}</h2>
		<p>{t('updates.goBack.intro', { version: goBack.to_version ?? '', when: dateTime(goBack.created_at) })}</p>
		<ol>
			<li>
				{t('updates.goBack.step1a')}
				<span class="cmd"><code data-testid="go-back-image">{IMAGE}:{tag}</code><Button variant="plain" size="sm" type="button" onclick={() => copy(`${IMAGE}:${tag}`)} aria-label={t('updates.goBack.copyImage')}>{t('updates.goBack.copy')}</Button></span>
				<Words key="updates.goBack.step1b" vars={{ tag }} />
			</li>
			<li><Words key="updates.goBack.step2" /></li>
			<li><Words key="updates.goBack.step3" vars={{ version: goBack.from_version }} /></li>
		</ol>
		<p class="fine"><Words key="updates.goBack.fine" /></p>
	</section>
{/if}

<style>
	.go-back p { margin: 0 0 10px; font-size: 14.5px; line-height: 1.5; color: var(--text); }
	.go-back ol { margin: 0 0 12px; padding-left: 1.4em; display: grid; gap: 8px; font-size: 14.5px; line-height: 1.5; }
	.go-back .fine { margin: 0; font-size: 12.5px; color: var(--text-muted); }
	.cmd { display: inline-flex; align-items: center; gap: 8px; flex-wrap: wrap; max-width: 100%; }
	code { color: var(--text); overflow-wrap: anywhere; }
</style>
