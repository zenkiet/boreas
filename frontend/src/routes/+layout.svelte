<script lang="ts">
	import Layout from '@/app/ui/Layout.svelte';

	let { children } = $props();
</script>

<Layout>{@render children()}</Layout>
