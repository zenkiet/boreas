<script lang="ts">
	import type { Snippet } from 'svelte';
	import { theme } from '#shared/theme';
	import '../styles/app.css';

	let { children }: { children: Snippet } = $props();

	$effect(() => {
		document.documentElement.dataset.theme = theme.resolved;
	});
</script>

{@render children()}
