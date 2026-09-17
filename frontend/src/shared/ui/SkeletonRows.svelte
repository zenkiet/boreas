<script lang="ts">
	let { count = 6, label = 'Loading' }: { count?: number; label?: string } = $props();
</script>

<div role="status" aria-label={label}>
	{#each { length: count }, i (i)}
		<div class="row-divider relative flex min-h-15 items-center gap-3 px-4 py-2.5">
			<span class="size-[7px] flex-none skeleton rounded-full"></span>
			<span class="flex min-w-0 flex-1 flex-col gap-2">
				<span class="h-[0.8125rem] skeleton" style:inline-size="{38 + ((i * 13) % 30)}%"></span>
				<span class="h-[0.6875rem] skeleton" style:inline-size="{55 + ((i * 17) % 35)}%"></span>
			</span>
			<span class="h-4 w-24 flex-none skeleton"></span>
		</div>
	{/each}
</div>
