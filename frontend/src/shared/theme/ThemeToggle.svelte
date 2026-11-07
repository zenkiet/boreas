<script lang="ts">
	import Moon from '@lucide/svelte/icons/moon';
	import Sun from '@lucide/svelte/icons/sun';
	import { theme } from './theme.svelte.ts';

	const label = $derived(
		theme.resolved === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'
	);
</script>

<button
	type="button"
	class="iconbtn"
	aria-label={label}
	title={label}
	onclick={() => theme.toggle()}
>
	{#if theme.resolved === 'dark'}<Sun size={20} />{:else}<Moon size={20} />{/if}
</button>
