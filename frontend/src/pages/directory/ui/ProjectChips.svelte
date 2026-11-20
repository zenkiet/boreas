<script module lang="ts">
	let refocus: string | undefined;
</script>

<script lang="ts">
	import type { Attachment } from 'svelte/attachments';
	import { directory } from '#entities/task';
	import { Count, roving, scrollHints } from '#shared/ui';
	import { showProject, urlState } from '../model/url-state.ts';

	const chips = $derived([
		{ slug: '', label: 'All projects', count: directory.tasks.length },
		...directory.projects.map(({ slug, tasks }) => ({ slug, label: slug, count: tasks.length }))
	]);

	const pick = (slug: string) => {
		refocus = slug;
		showProject(slug, { reset: false });
	};

	const reveal: Attachment<HTMLElement> = (chip) => {
		chip.scrollIntoView({ block: 'nearest', inline: 'nearest', container: 'nearest' });
		if (refocus !== chip.dataset.slug) return;
		refocus = undefined;
		chip.focus();
	};
</script>

<div
	class="-mx-8 mb-3 flex scroll-fade scroll-px-8 [scrollbar-width:none] gap-1.5 overflow-x-auto px-8 pb-0.5 max-sm:-mx-4 max-sm:px-4 lg:hidden"
	role="toolbar"
	aria-label="Filter by project"
	focusgroup="toolbar wrap"
	{@attach roving}
	{@attach scrollHints}
>
	{#each chips as chip (chip.slug)}
		{@const pressed = urlState.project === chip.slug}
		<button
			type="button"
			class="chip chip-project"
			aria-pressed={pressed}
			data-slug={chip.slug}
			onclick={() => pick(chip.slug)}
			{@attach pressed ? reveal : undefined}
		>
			{chip.label}<span data-count><Count value={chip.count} /></span>
		</button>
	{/each}
</div>
