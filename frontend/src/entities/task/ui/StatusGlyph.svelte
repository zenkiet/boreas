<script lang="ts">
	import type { ClassValue } from 'svelte/elements';
	import type { DevStatus } from '#shared/api';

	let {
		status,
		size = 16,
		label,
		class: className
	}: { status: DevStatus; size?: number; label?: string; class?: ClassValue } = $props();
</script>

<svg
	viewBox="0 0 16 16"
	width={size}
	height={size}
	class={['block flex-none', className]}
	role={label ? 'img' : undefined}
	aria-label={label}
	aria-hidden={label ? undefined : 'true'}
>
	{#if status === 'blocked'}
		<g class="text-blocked">
			<circle cx="8" cy="8" r="7" fill="currentColor" />
			<rect class="fill-mark" x="4.25" y="7.1" width="7.5" height="1.8" rx="0.9" />
		</g>
	{:else if status === 'in_progress'}
		<g class="text-progress">
			<circle cx="8" cy="8" r="6.2" fill="none" stroke="currentColor" stroke-width="1.6" />
			<path d="M8 3.6a4.4 4.4 0 0 1 0 8.8z" fill="currentColor" />
		</g>
	{:else}
		<g class="text-ready">
			<circle cx="8" cy="8" r="7" fill="currentColor" />
			<path
				class="stroke-mark"
				d="m4.9 8.2 2 2 4.2-4.3"
				fill="none"
				stroke-width="1.7"
				stroke-linecap="round"
				stroke-linejoin="round"
			/>
		</g>
	{/if}
</svg>
