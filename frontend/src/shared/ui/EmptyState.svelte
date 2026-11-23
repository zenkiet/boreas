<script lang="ts">
	import type { LucideIcon } from '@lucide/svelte';
	import type { Snippet } from 'svelte';

	let {
		icon: Icon,
		tone = 'neutral',
		title,
		description,
		children
	}: {
		icon: LucideIcon;
		tone?: 'neutral' | 'bad';
		title: string;
		description: string;
		children?: Snippet;
	} = $props();
</script>

<div
	class="flex flex-col items-center gap-2 px-6 py-14 text-center"
	role={tone === 'bad' ? 'alert' : undefined}
>
	<span
		class={[
			'mb-1 grid size-11 pop place-items-center rounded-nest',
			tone === 'bad' ? 'bg-blocked-soft text-blocked' : 'bg-fill-1 text-label-3'
		]}
	>
		<Icon size={20} />
	</span>
	<h2 class="text-headline">{title}</h2>
	<p class="max-w-105 text-subhead/[1.4] text-label-2">{description}</p>
	{#if children}
		<div class="mt-2 flex flex-wrap justify-center gap-2">{@render children()}</div>
	{/if}
</div>
