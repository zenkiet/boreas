<script lang="ts">
	import Icon from './Icon.svelte';

	let {
		value = $bindable(''),
		label,
		placeholder = 'Search'
	}: { value?: string; label: string; placeholder?: string } = $props();
</script>

<div class="field">
	<Icon name="search" size={17} class="flex-none text-tertiary" />
	<input
		type="search"
		aria-label={label}
		{placeholder}
		bind:value
		class="min-w-0 flex-1 bg-transparent text-[0.9375rem] text-primary outline-none placeholder:text-tertiary"
	/>
	{#if value}
		<button type="button" class="clear" aria-label="Clear search" onclick={() => (value = '')}>
			<Icon name="x" size={15} />
		</button>
	{/if}
</div>

<style>
	.field {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		block-size: 2.5rem;
		padding-inline: 0.875rem;
		border-radius: 999px;
		background: var(--app-glass-fill);
		box-shadow:
			inset 0 0 0 1px var(--tui-border-normal),
			inset 0 1px 0 var(--app-glass-bevel),
			inset 0 0 0.75rem var(--app-glass-glow);
		transition: box-shadow var(--tui-duration);
	}

	.field:focus-within {
		background: transparent;
		box-shadow: inset 0 0 0 2px var(--tui-background-accent-1);
	}

	input::-webkit-search-cancel-button {
		appearance: none;
	}

	.clear {
		display: inline-flex;
		flex: none;
		align-items: center;
		justify-content: center;
		inline-size: 1.25rem;
		block-size: 1.25rem;
		border: 0;
		border-radius: 999px;
		padding: 0;
		background: var(--tui-background-neutral-2);
		color: var(--tui-text-secondary);
		cursor: pointer;
	}
</style>
