<script lang="ts">
	import { highlight } from '../model/search.ts';

	let { text, needle }: { text: string; needle: string } = $props();
</script>

{#each highlight(text, needle) as part, i (i)}{#if part.hit}<mark
			class="rounded-[3px] bg-accent/22 text-inherit">{part.text}</mark
		>{:else}{part.text}{/if}{/each}
