<script lang="ts">
	import ArrowUpRight from '@lucide/svelte/icons/arrow-up-right';
	import Lock from '@lucide/svelte/icons/lock';
	import Monitor from '@lucide/svelte/icons/monitor';
	import Smartphone from '@lucide/svelte/icons/smartphone';
	import { page } from '$app/state';
	import { appUrl, taskKey } from '#shared/api';
	import type { DirectoryTask } from '#shared/api';
	import { CopyButton } from '#shared/ui';
	import { StatusBadge } from '#entities/task';
	import ContainerFlag from './ContainerFlag.svelte';

	let { task }: { task: DirectoryTask } = $props();

	const url = $derived(appUrl(task));
</script>

<div class="mb-[18px] flex flex-col gap-2">
	<h2 class="font-mono text-mono-title">{task.name}</h2>
	<p class={['text-body/[23px]', task.description ? 'text-label' : 'text-label-3']}>
		{task.description || 'No description'}
	</p>
	<div class="mt-1 flex flex-wrap gap-1.5">
		<StatusBadge status={task.devStatus} />
		<ContainerFlag status={task.status} large />
		<span class="flag flag-lg">
			{#if task.formFactor === 'mobile'}
				<Smartphone size={12} />Mobile app
			{:else}
				<Monitor size={12} />Web app
			{/if}
		</span>
		{#if task.locked}
			<span class="flag flag-lg bg-info-soft text-info"><Lock size={12} />Password</span>
		{/if}
	</div>
</div>
<div class="flex gap-2">
	<a class="btn flex-1" href={url} target="_blank" rel="external noopener">
		<ArrowUpRight size={16} />Open app
	</a>
	<CopyButton text={new URL(`?task=${taskKey(task)}`, page.url.href).href} class="flex-1" />
</div>
<p class="mt-2.5 font-mono text-caption font-normal wrap-anywhere text-label-3">
	{url.replace(/^https?:\/\//, '')}
</p>
