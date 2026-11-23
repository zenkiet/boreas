<script lang="ts">
	import Layers2 from '@lucide/svelte/icons/layers-2';
	import { resolve } from '$app/paths';

	let {
		slug,
		link = false,
		title,
		icon = false
	}: { slug: string; link?: boolean; title?: string; icon?: boolean } = $props();

	const TAG =
		'inline-flex min-h-6 max-w-full items-center gap-[5px] rounded-label bg-fill-1 px-[7px] font-mono text-caption/none font-medium whitespace-nowrap text-label-2 no-underline';
</script>

{#snippet content()}
	{#if icon}<Layers2 size={12} class="flex-none" />{/if}<span class="truncate">{slug}</span>
{/snippet}

{#if link}
	<a
		href={resolve('/(directory)/[project]', { project: slug })}
		title={title ?? slug}
		class={[
			TAG,
			'cursor-pointer transition-colors duration-200 ease-out hover:bg-fill-2 hover:text-label'
		]}
	>
		{@render content()}
	</a>
{:else}
	<span title={title ?? slug} class={TAG}>{@render content()}</span>
{/if}
