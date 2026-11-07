<script lang="ts">
	import Layers2 from '@lucide/svelte/icons/layers-2';
	import { resolve } from '$app/paths';

	let {
		slug,
		link = false,
		title,
		icon = false,
		onclick
	}: {
		slug: string;
		link?: boolean;
		title?: string;
		icon?: boolean;
		onclick?: (event: MouseEvent) => void;
	} = $props();

	const TAG =
		'relative z-1 inline-flex h-6 max-w-full items-center gap-[5px] rounded-label bg-fill-1 px-[7px] text-caption/none font-medium whitespace-nowrap text-label-2 no-underline';
	const ACTIVE =
		'cursor-pointer transition-colors duration-200 ease-snap hover:bg-fill-2 hover:text-label';
</script>

{#snippet content()}
	{#if icon}<Layers2 size={12} class="flex-none" />{/if}<span class="truncate">{slug}</span>
{/snippet}

{#if link}
	<a
		href={resolve('/(directory)/[project]', { project: slug })}
		{title}
		class={[TAG, ACTIVE, 'font-mono']}
	>
		{@render content()}
	</a>
{:else if onclick}
	<button type="button" {title} {onclick} class={[TAG, ACTIVE]}>{@render content()}</button>
{:else}
	<span class={[TAG, 'font-mono']}>{@render content()}</span>
{/if}
