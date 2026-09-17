<script lang="ts" module>
	const PATHS = {
		'chevron-right': ['M9 18l6-6-6-6'],
		'chevron-left': ['M15 18l-6-6 6-6'],
		search: ['M11 19a8 8 0 1 0 0-16 8 8 0 0 0 0 16z', 'm21 21-4.3-4.3'],
		x: ['M18 6 6 18', 'm6 6 12 12'],
		sun: [
			'M12 16a4 4 0 1 0 0-8 4 4 0 0 0 0 8z',
			'M12 2v2',
			'M12 20v2',
			'm4.93 4.93 1.41 1.41',
			'm17.66 17.66 1.41 1.41',
			'M2 12h2',
			'M20 12h2',
			'm6.34 17.66-1.41 1.41',
			'm19.07 4.93-1.41 1.41'
		],
		moon: ['M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9z'],
		monitor: [
			'M21 3H3a1 1 0 0 0-1 1v11a1 1 0 0 0 1 1h18a1 1 0 0 0 1-1V4a1 1 0 0 0-1-1z',
			'M8 21h8',
			'M12 17v4'
		],
		layers: [
			'm12.83 2.18a2 2 0 0 0-1.66 0L2.6 6.08a1 1 0 0 0 0 1.83l8.58 3.91a2 2 0 0 0 1.66 0l8.58-3.9a1 1 0 0 0 0-1.83z',
			'm6.08 12.5-3.5 1.6a1 1 0 0 0 0 1.81l8.6 3.91a2 2 0 0 0 1.65 0l8.58-3.9a1 1 0 0 0 0-1.83l-3.5-1.59'
		],
		box: ['m21 8-9-5-9 5v8l9 5 9-5z', 'm3.3 7 8.7 5 8.7-5', 'M12 22V12'],
		clock: ['M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20z', 'M12 6v6l4 2'],
		'refresh-cw': [
			'M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8',
			'M21 3v5h-5',
			'M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16',
			'M8 16H3v5'
		],
		'triangle-alert': [
			'm21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3',
			'M12 9v4',
			'M12 17h.01'
		],
		'external-link': [
			'M15 3h6v6',
			'M10 14 21 3',
			'M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h6'
		],
		compass: [
			'M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20z',
			'm16.24 7.76-2.12 6.36-6.36 2.12 2.12-6.36z'
		],
		power: ['M18.36 6.64a9 9 0 1 1-12.73 0', 'M12 2v10']
	} as const;

	export type IconName = keyof typeof PATHS;
</script>

<script lang="ts">
	let {
		name,
		size = 16,
		class: className = ''
	}: { name: IconName; size?: number; class?: string } = $props();
</script>

<svg
	class={className}
	width={size}
	height={size}
	viewBox="0 0 24 24"
	fill="none"
	stroke="currentColor"
	stroke-width="1.75"
	stroke-linecap="round"
	stroke-linejoin="round"
	aria-hidden="true"
>
	{#each PATHS[name] as d (d)}
		<path {d} />
	{/each}
</svg>
