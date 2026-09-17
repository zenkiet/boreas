<script lang="ts">
	let {
		items,
		active = $bindable(0),
		label
	}: {
		items: readonly { label: string; count: number }[];
		active?: number;
		label: string;
	} = $props();
</script>

<div class="seg" role="tablist" aria-label={label}>
	<span
		class="thumb"
		style:inline-size="calc({100 / items.length}% - 0.1875rem)"
		style:transform="translateX(calc({active * 100}% + {active * 0.1875}rem))"
		aria-hidden="true"
	></span>
	{#each items as item, index (item.label)}
		<button
			type="button"
			role="tab"
			aria-selected={index === active}
			class:active={index === active}
			onclick={() => (active = index)}
		>
			{item.label}
			<span class="count tabular">{item.count}</span>
		</button>
	{/each}
</div>

<style>
	.seg {
		position: relative;
		display: flex;
		block-size: 2.5rem;
		padding: 0.1875rem;
		border-radius: 999px;
		background: var(--app-chrome-bg);
		-webkit-backdrop-filter: var(--app-chrome-filter);
		backdrop-filter: var(--app-chrome-filter);
		box-shadow:
			0 0.5rem 2rem rgba(0, 0, 0, 0.09),
			inset 0 0 0.75rem var(--app-chrome-glow);
	}

	.thumb {
		position: absolute;
		inset-block: 0.1875rem;
		inset-inline-start: 0.1875rem;
		border-radius: 999px;
		background: var(--app-segment-thumb);
		box-shadow: var(--app-segment-shadow);
		transition: transform 0.36s cubic-bezier(0.34, 1.4, 0.5, 1);
	}

	button {
		position: relative;
		z-index: 1;
		display: inline-flex;
		flex: 1;
		align-items: center;
		justify-content: center;
		gap: 0.3125rem;
		min-inline-size: 0;
		border: 0;
		border-radius: 999px;
		padding-inline: 0.5rem;
		background: none;
		font: inherit;
		font-size: 0.875rem;
		font-weight: 500;
		white-space: nowrap;
		color: var(--tui-text-secondary);
		cursor: pointer;
		transition: color var(--tui-duration);
	}

	.active {
		color: var(--tui-text-primary);
		font-weight: 600;
	}

	.count {
		font-size: 0.75rem;
		color: var(--tui-text-tertiary);
	}

	.active .count {
		color: var(--tui-text-secondary);
	}
</style>
