<script lang="ts">
	import type { Snippet } from 'svelte';
	import { theme } from '#shared/theme';
	import '../styles/app.css';

	let { children }: { children: Snippet } = $props();

	$effect(() => {
		document.querySelector<HTMLMetaElement>('meta[name="color-scheme"]')!.content =
			theme.chosen ?? 'light dark';
	});
</script>

{@render children()}
