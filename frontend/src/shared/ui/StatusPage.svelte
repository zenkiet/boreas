<script lang="ts">
	import type { Snippet } from 'svelte';
	import Icon, { type IconName } from './Icon.svelte';

	let {
		code,
		icon,
		tone = 'neutral',
		title,
		description,
		path,
		children
	}: {
		code: string;
		icon: IconName;
		tone?: 'neutral' | 'warning' | 'negative';
		title: string;
		description: string;
		path?: string;
		children?: Snippet;
	} = $props();
</script>

<div class="grid w-full max-w-128 justify-items-center gap-4 text-center" data-tone={tone}>
	<span class="badge inline-flex size-14 items-center justify-center rounded-[1.125rem]">
		<Icon name={icon} size={26} />
	</span>

	<div class="grid gap-2">
		<span class="code font-mono">{code}</span>
		<h1 class="text-[1.625rem] leading-tight font-bold tracking-[-0.02em] text-primary">{title}</h1>
		<p class="text-[0.9375rem] leading-relaxed text-secondary">{description}</p>
	</div>

	{#if path}
		<code class="chip font-mono">{path}</code>
	{/if}

	{#if children}{@render children()}{/if}
</div>

<style>
	.badge {
		background: var(--tui-status-neutral-pale);
		color: var(--tui-status-neutral);
	}

	[data-tone='warning'] .badge {
		background: var(--tui-status-warning-pale);
		color: var(--tui-status-warning);
	}

	[data-tone='negative'] .badge {
		background: var(--tui-status-negative-pale);
		color: var(--tui-status-negative);
	}

	.code {
		font-size: 0.8125rem;
		font-weight: 600;
		letter-spacing: 0.12em;
		color: var(--tui-text-tertiary);
	}

	.chip {
		display: inline-block;
		max-inline-size: 100%;
		overflow: hidden;
		padding: 0.4375rem 0.875rem;
		border-radius: 999px;
		background: var(--tui-background-neutral-1);
		font-size: 0.8125rem;
		color: var(--tui-text-secondary);
		text-overflow: ellipsis;
		white-space: nowrap;
	}
</style>
