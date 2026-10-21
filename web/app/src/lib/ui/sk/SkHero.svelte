<script lang="ts">
	// A Billboard or title hero placeholder: the same height, with the title, two lines and
	// the buttons where the real ones sit.
	let { size = 'billboard' }: { size?: 'billboard' | 'title' | 'person' } = $props();
</script>

<div class="sk-hero {size}" aria-hidden="true">
	<span class="sk bg"></span>
	<div class="copy">
		<span class="sk sk-line t"></span>
		<span class="sk sk-line l"></span>
		<span class="sk sk-line l2"></span>
		<div class="btns"><span class="sk b"></span><span class="sk b r"></span><span class="sk b r"></span></div>
	</div>
</div>

<style>
	.sk-hero { position: relative; border-radius: var(--hero-r, var(--r-panel)); overflow: hidden; }
	.billboard { --hero-r: 24px; margin-top: 8px; min-height: max(380px, min(77svh, calc((100vw - var(--gutter) * 2) / 2.18))); }
	.title { min-height: max(360px, min(70svh, calc((100vw - var(--gutter) * 2) / 2.2))); }
	.person { min-height: 280px; }
	.bg { position: absolute; inset: 0; border-radius: inherit; }
	.copy { position: absolute; left: clamp(20px, 4vw, 56px); bottom: clamp(24px, 5vw, 56px); width: min(520px, 70%); display: flex; flex-direction: column; gap: 12px; }
	.copy .sk { background-color: color-mix(in oklab, var(--sk-base), var(--text) 6%); }
	.t { height: clamp(36px, 5vw, 64px); width: 80%; border-radius: 10px; margin-bottom: 6px; }
	.l { width: 100%; }
	.l2 { width: 70%; }
	.btns { display: flex; gap: 10px; margin-top: 8px; }
	.b { width: 132px; height: 44px; border-radius: var(--r-pill); }
	.b.r { width: 44px; }
	@media (max-width: 767px) { .billboard { --hero-r: 16px; min-height: min(560px, 120vw); } }
</style>
