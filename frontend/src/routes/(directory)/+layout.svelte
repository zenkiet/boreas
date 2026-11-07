<script lang="ts">
	import { DirectoryShell } from '#pages/directory';

	let { children } = $props();
</script>

<DirectoryShell>{@render children()}</DirectoryShell>
