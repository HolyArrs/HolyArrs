<!-- The files a scanned show or artist folder holds, folded under a one-line summary. -->
<script lang="ts">
	import { albumFolderLine, cutFilesLine, episodeFileLine, fileName, musicFolderLine, showFolderLine, trackFileLine, type AdoptCandidate } from './organise.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	let { f }: { f: AdoptCandidate } = $props();
</script>

{#if f.albums?.length}
	<details class="eps" data-testid="music-albums">
		<summary class="xs">{musicFolderLine(f)}</summary>
		<ul class="files albums">
			{#each f.albums as al (al.folder)}
				<li class="album" data-testid="music-album">
					<span class="small"><span class="mono path">{al.folder}</span></span>
					<span class="xs muted">{albumFolderLine(al)}</span>
					<ul class="files">
						{#each al.files as tf (tf.path)}
							<li class="xs" class:left={!tf.track}>
								<span class="mono path">{fileName(tf.path)}</span>
								<span class="muted">{trackFileLine(tf)}</span>
							</li>
						{/each}
						{#if cutFilesLine(al)}<li class="xs muted">{cutFilesLine(al)}</li>{/if}
					</ul>
				</li>
			{/each}
			{#each f.files ?? [] as ef (ef.path)}
				<li class="xs left">
					<span class="mono path">{fileName(ef.path)}</span>
					<span class="muted">{episodeFileLine(ef)}</span>
				</li>
			{/each}
			{#if f.cut}<li class="xs muted">{t('library.import.moreLoose')}</li>{/if}
		</ul>
	</details>
{:else if f.files?.length}
	<details class="eps" data-testid="show-files">
		<summary class="xs">{showFolderLine(f)}</summary>
		<ul class="files">
			{#each f.files as ef (ef.path)}
				<li class="xs" class:left={!ef.episodes.length}>
					<span class="mono path">{fileName(ef.path)}</span>
					<span class="muted">{episodeFileLine(ef)}</span>
				</li>
			{/each}
		</ul>
	</details>
{/if}

<style>
	.eps { min-width: 0; }
	.eps summary { display: flex; align-items: center; min-height: 32px; color: var(--text-muted); cursor: pointer; }
	.eps summary:hover { color: var(--text); }
	.eps summary:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 6px; }
	.files { list-style: none; padding: 0; margin: 0; }
	.files li { display: flex; flex-wrap: wrap; gap: 2px var(--s-2); padding: 2px 0; }
	.files li.left .path { color: var(--text-muted); }
	.files li.album { flex-direction: column; padding: var(--s-2) 0; }
	.albums > li + li { border-top: 1px solid var(--line); }
	.path { overflow-wrap: anywhere; }
</style>
