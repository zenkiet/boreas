<script lang="ts">
	import type { Snippet } from 'svelte';
	import Icon, { type IconName } from './Icon.svelte';

	let {
		icon = 'box',
		title,
		description,
		tone = 'neutral',
		children
	}: {
		icon?: IconName;
		title: string;
		description: string;
		tone?: 'neutral' | 'negative';
		children?: Snippet;
	} = $props();
</script>

<div class="flex flex-col items-center gap-2 px-6 py-12 text-center" data-tone={tone}>
	<span
		class="badge mb-1 inline-flex size-11 items-center justify-center rounded-[var(--radius-md)]"
	>
		<Icon name={icon} size={22} />
	</span>
	<h3 class="text-[1.0625rem] font-semibold text-primary">{title}</h3>
	<p class="max-w-104 text-[0.9375rem] leading-normal text-secondary">{description}</p>
	{#if children}<div class="mt-2">{@render children()}</div>{/if}
</div>

<style>
	.badge {
		background: var(--tui-background-neutral-1);
		color: var(--tui-text-tertiary);
	}

	[data-tone='negative'] .badge {
		background: var(--tui-status-negative-pale);
		color: var(--tui-status-negative);
	}
</style>
