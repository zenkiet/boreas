<script lang="ts" generics="T extends string">
	import type { LucideIcon } from '@lucide/svelte';

	interface Option {
		value: T;
		label: string;
		icon?: LucideIcon;
	}

	let {
		options,
		value = $bindable(),
		label,
		iconOnly = false
	}: { options: readonly Option[]; value: T; label: string; iconOnly?: boolean } = $props();

	const id = $props.id();
</script>

<div class="seg" role="group" aria-label={label}>
	{#each options as item (item.value)}
		<button
			type="button"
			class={['seg-btn', iconOnly && 'w-[34px] px-0']}
			aria-pressed={value === item.value}
			aria-label={iconOnly ? item.label : undefined}
			title={iconOnly ? item.label : undefined}
			onclick={() => (value = item.value)}
		>
			{#if value === item.value}
				<span class="seg-thumb" style:view-transition-name="seg-{id}"></span>
			{/if}
			{#if item.icon}<item.icon size={16} />{/if}
			{#if !iconOnly}{item.label}{/if}
		</button>
	{/each}
</div>
