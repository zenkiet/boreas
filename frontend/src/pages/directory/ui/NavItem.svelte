<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { ResolvedPathname } from '$app/types';
	import { Count } from '#shared/ui';

	let {
		href,
		current,
		count,
		replace = false,
		children
	}: {
		href: ResolvedPathname;
		current: boolean;
		count?: number;
		replace?: boolean;
		children: Snippet;
	} = $props();
</script>

<a
	{href}
	class="group flex h-[34px] w-full items-center gap-2.5 rounded-nest px-2.5 text-[14px] text-label transition-colors duration-200 ease-out hover:bg-fill-1 aria-[current=page]:bg-accent-soft aria-[current=page]:font-semibold aria-[current=page]:text-accent-text"
	aria-current={current ? 'page' : undefined}
	data-sveltekit-replacestate={replace || undefined}
>
	{@render children()}
	<span
		class="text-caption font-normal text-label-3 tabular-nums group-aria-[current=page]:font-semibold group-aria-[current=page]:text-inherit"
		>{#if count !== undefined}<Count value={count} />{/if}</span
	>
</a>
