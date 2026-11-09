<script lang="ts">
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
			toasts.push('Text copied', 'ok');
		} catch {
			toasts.push("Couldn't copy", 'fail', { body: 'Select it instead' });
		}
	}
</script>

{#if goBack?.from_version}
	{@const tag = imageTag(goBack.from_version)}
	<section class="st-plate go-back" aria-labelledby="s-goback" data-testid="go-back">
		<h2 id="s-goback">Go back to {goBack.from_version}</h2>
		<p>HolyArrs kept a copy of its data from just before the update to {goBack.to_version} ({dateTime(goBack.created_at)}). To go back:</p>
		<ol>
			<li>
				Change the image back to
				<span class="cmd"><code data-testid="go-back-image">{IMAGE}:{tag}</code><Button variant="plain" size="sm" type="button" onclick={() => copy(`${IMAGE}:${tag}`)} aria-label="Copy the image name">Copy</Button></span>
				If you pull from another registry, keep its name and change only the tag to <code>{tag}</code>.
			</li>
			<li>Start the container again with that image (with Compose: <code>docker compose up -d</code>).</li>
			<li>{goBack.from_version} sees that its data is newer and shows a page instead. There, type the one-time code from <code>docker logs &lt;container&gt;</code> and press <b>Put back the data from before the update</b>. HolyArrs then starts on that data.</li>
		</ol>
		<p class="fine">Later changes move to backups/before-restore-&lt;time&gt;. From the command line: <code>docker exec &lt;container&gt; /holyarrs restore --pre-update</code>, then <code>docker restart &lt;container&gt;</code>.</p>
	</section>
{/if}

<style>
	.go-back p { margin: 0 0 10px; font-size: 14.5px; line-height: 1.5; color: var(--text); }
	.go-back ol { margin: 0 0 12px; padding-left: 1.4em; display: grid; gap: 8px; font-size: 14.5px; line-height: 1.5; }
	.go-back .fine { margin: 0; font-size: 12.5px; color: var(--text-muted); }
	.cmd { display: inline-flex; align-items: center; gap: 8px; flex-wrap: wrap; max-width: 100%; }
	code { color: var(--text); overflow-wrap: anywhere; }
</style>
