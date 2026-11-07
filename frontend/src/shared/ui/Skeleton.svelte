<script lang="ts">
	import type { ClassValue } from 'svelte/elements';

	let {
		class: className,
		width,
		height
	}: { class?: ClassValue; width?: string; height?: string } = $props();
</script>

<span class={['skeleton', className]} style:width style:height aria-hidden="true"></span>
