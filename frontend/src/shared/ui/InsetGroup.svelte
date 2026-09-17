<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		label,
		trailing = '',
		children
	}: { label: string; trailing?: string; children: Snippet } = $props();
</script>

<section>
	<div class="flex items-baseline justify-between gap-3 px-1 pb-1.5">
		<h2 class="text-[0.8125rem] leading-[1.4] font-medium text-tertiary">{label}</h2>
		{#if trailing}
			<span class="text-[0.8125rem] whitespace-nowrap text-tertiary tabular" aria-live="polite">
				{trailing}
			</span>
		{/if}
	</div>
	<div class="overflow-hidden rounded-[var(--radius-lg)] bg-base">{@render children()}</div>
</section>
