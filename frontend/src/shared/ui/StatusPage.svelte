<script lang="ts">
	import type { LucideIcon } from '@lucide/svelte';
	import type { Snippet } from 'svelte';
	import PageTop from './PageTop.svelte';

	let {
		icon: Icon,
		tone = 'neutral',
		code,
		title,
		body,
		path,
		actions,
		extra
	}: {
		icon: LucideIcon;
		tone?: 'neutral' | 'warn' | 'bad';
		code: string;
		title: string;
		body: string;
		path?: string;
		actions?: Snippet;
		extra?: Snippet;
	} = $props();

	const TONE = {
		neutral: 'bg-neutral-soft text-neutral',
		warn: 'bg-progress-soft text-progress',
		bad: 'bg-blocked-soft text-blocked'
	};
</script>

<div class="flex min-h-dvh flex-col">
	<PageTop />
	<main class="flex flex-1 justify-center px-5 pt-14 pb-16 max-sm:px-4 max-sm:pt-8 max-sm:pb-10">
		<div class="flex w-full max-w-140 flex-col items-center gap-4 text-center">
			<span class={['grid size-14 place-items-center rounded-box', TONE[tone]]}>
				<Icon size={28} />
			</span>
			<div class="flex flex-col items-center gap-2">
				<span class="font-mono text-caption tracking-[0.12em] text-label-3">{code}</span>
				<h1 class="text-title1 wrap-anywhere max-sm:text-[1.5rem]/[1.875rem]">{title}</h1>
				<p class="max-w-120 text-subhead/[22px] text-label-2">{body}</p>
			</div>
			{#if path}
				<code
					class="max-w-full truncate rounded-label bg-fill-1 px-3 py-1.5 font-mono text-footnote font-medium text-label-2"
				>
					{path}
				</code>
			{/if}
			{#if actions}
				<div class="flex flex-wrap justify-center gap-2">{@render actions()}</div>
			{/if}
			{@render extra?.()}
		</div>
	</main>
</div>
