<script lang="ts">
	import { resolve } from '$app/paths';
	import { ThemeToggle } from '#shared/theme';
	import Brand from './Brand.svelte';
</script>

<header
	class="z-5 flex min-h-14 items-center gap-2 border-b border-sep bg-surface ps-5 pe-4 max-sm:sticky max-sm:top-0"
>
	<Brand />
	<a class="ms-auto btn btn-quiet btn-sm" href={resolve('/(directory)')}>All tasks</a>
	<ThemeToggle />
</header>
