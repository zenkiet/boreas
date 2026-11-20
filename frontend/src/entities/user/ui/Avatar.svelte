<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { initials, toneOf } from '../model/avatar.ts';

	let {
		username,
		src,
		size = 24,
		class: className,
		...rest
	}: HTMLAttributes<HTMLSpanElement> & {
		username?: string;
		src?: string;
		size?: 20 | 24 | 28 | 40;
	} = $props();

	let failed = $state<string>();

	const SIZE = {
		20: 'size-5 text-[0.53125rem]',
		24: 'size-6 text-[0.625rem]',
		28: 'size-7 text-[0.6875rem]',
		40: 'size-10 text-[0.9375rem]'
	};

	const TONE = ['bg-av-0', 'bg-av-1', 'bg-av-2', 'bg-av-3', 'bg-av-4', 'bg-av-5', 'bg-av-6'];
</script>

{#if username}
	<span
		class={[
			'relative inline-grid flex-none place-items-center rounded-full leading-none font-semibold tracking-[0.02em] text-white uppercase',
			SIZE[size],
			TONE[toneOf(username)],
			className
		]}
		{...rest}
		aria-hidden="true"
	>
		{initials(username)}
		{#if src && src !== failed}
			<img
				{src}
				alt=""
				loading="lazy"
				class="absolute inset-0 size-full rounded-full object-cover"
				onerror={() => (failed = src)}
			/>
		{/if}
	</span>
{:else}
	<span
		class={[
			'inline-block flex-none rounded-full border-[1.5px] border-dashed border-line',
			SIZE[size],
			className
		]}
		aria-hidden="true"
	></span>
{/if}
