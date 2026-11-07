<script lang="ts">
	import { resolve } from '$app/paths';

	const uid = $props.id();
</script>

<a
	href={resolve('/(directory)')}
	class="inline-flex items-center gap-2 text-headline font-bold tracking-[-0.01em] text-label no-underline"
	aria-label="Boreas, all tasks"
>
	<svg viewBox="0 0 1024 1024" class="size-7 flex-none" aria-hidden="true">
		<defs>
			<linearGradient id="{uid}-logo" x1="0" y1="0" x2="0" y2="1">
				<stop offset="0" stop-color="#47A8FF" />
				<stop offset="0.4" stop-color="#2096FF" />
				<stop offset="0.675" stop-color="#0088FF" />
			</linearGradient>
		</defs>
		<rect width="1024" height="1024" rx="229" fill="url(#{uid}-logo)" />
		<g fill="#ffffff">
			<path
				d="M645.7 311.4c0 94.75-76.83 171.49-171.58 171.49-4.1 0-8.12-.17-12.12-.52l36.36-104.57h.08c27.15-9.9 46.43-35.94 46.43-66.41 0-39.1-31.67-70.76-70.76-70.76s-70.68 31.67-70.68 70.76v574.72H302.6V311.4c0-32.86 9.3-63.6 25.35-89.71 30.13-49.09 84.26-81.86 146.14-81.86 94.75 0 171.58 76.83 171.58 171.58Z"
			/>
			<path
				d="M801.49 668.37c0 99.96-81 180.96-180.96 180.96-9.38 0-18.61-.69-27.57-2.05v-97.31c8.71 2.91 17.92 4.52 27.57 4.52 47.54 0 86.13-38.58 86.13-86.13s-38.58-86.04-86.13-86.04-86.13 38.5-86.13 86.04v217.76h-94.83V668.37c0-68.71 38.33-128.54 94.83-159.19 25.61-13.92 54.97-21.77 86.13-21.77 99.96 0 180.96 81.1 180.96 180.96Z"
			/>
		</g>
	</svg>
	<span>Boreas</span>
</a>
