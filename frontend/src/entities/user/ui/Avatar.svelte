<script lang="ts">
	import type { ClassValue } from 'svelte/elements';
	import { initials, toneOf } from '../model/avatar.ts';

	let {
		username,
		size = 24,
		class: className
	}: { username?: string; size?: 20 | 24 | 28 | 40; class?: ClassValue } = $props();

	const SIZE = {
		20: 'size-5 text-[8.5px]',
		24: 'size-6 text-[10px]',
		28: 'size-7 text-[11px]',
		40: 'size-10 text-[15px]'
	};

	const TONE = ['bg-av-0', 'bg-av-1', 'bg-av-2', 'bg-av-3', 'bg-av-4', 'bg-av-5', 'bg-av-6'];
</script>

{#if username}
	<span
		class={[
			'inline-grid flex-none place-items-center rounded-full leading-none font-semibold tracking-[0.02em] text-white uppercase',
			SIZE[size],
			TONE[toneOf(username)],
			className
		]}
		aria-hidden="true">{initials(username)}</span
	>
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
