<script lang="ts">
	import type { Snippet } from 'svelte';
	import { theme } from '#shared/theme';
	import favicon from '../favicon.svg';
	import '../styles/app.css';

	let { children }: { children: Snippet } = $props();

	$effect(() => {
		document.documentElement.dataset.theme = theme.resolved;
	});
</script>

<svelte:head><link rel="icon" href={favicon} /></svelte:head>

{@render children()}
