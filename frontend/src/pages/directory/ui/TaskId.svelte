<script lang="ts">
	import Lock from '@lucide/svelte/icons/lock';
	import Smartphone from '@lucide/svelte/icons/smartphone';
	import type { ClassValue } from 'svelte/elements';
	import type { PublicTask } from '#shared/api';
	import Highlight from './Highlight.svelte';

	let {
		task,
		needle = '',
		idle = false,
		class: className
	}: { task: PublicTask; needle?: string; idle?: boolean; class?: ClassValue } = $props();
</script>

<span
	class={[
		'flex min-w-0 items-center gap-1.5 font-mono text-footnote font-semibold text-label',
		className
	]}
>
	<span class={['truncate', idle && 'text-label-2']}><Highlight text={task.name} {needle} /></span>
	{#if task.formFactor === 'mobile'}
		<Smartphone size={14} class="flex-none text-label-3" role="img" aria-label="Mobile app" />
	{/if}
	{#if task.locked}
		<Lock size={14} class="flex-none text-label-3" role="img" aria-label="Password protected" />
	{/if}
</span>
