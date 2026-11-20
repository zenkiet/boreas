<script module lang="ts">
	const hits = typeof CSS !== 'undefined' && CSS.highlights ? new Highlight() : undefined;
	if (hits) CSS.highlights.set('search-hit', hits);
</script>

<script lang="ts">
	import type { Attachment } from 'svelte/attachments';
	import { highlight } from '../model/search.ts';

	let { text, needle }: { text: string; needle: string } = $props();

	const mark =
		(parts: ReturnType<typeof highlight>): Attachment<HTMLElement> =>
		(span) => {
			const node = span.firstChild;
			if (!hits || !(node instanceof Text)) return;
			const ranges: Range[] = [];
			let at = 0;
			for (const part of parts) {
				const end = Math.min(at + part.text.length, node.length);
				if (part.hit) {
					const range = new Range();
					range.setStart(node, at);
					range.setEnd(node, end);
					ranges.push(range);
					hits.add(range);
				}
				at = end;
			}
			return () => ranges.forEach((range) => hits.delete(range));
		};
</script>

{#if hits}
	<span {@attach mark(highlight(text, needle))}>{text}</span>
{:else}
	{#each highlight(text, needle) as part, i (i)}{#if part.hit}<mark
				class="rounded-[3px] bg-accent/22 text-inherit underline decoration-accent"
			>
				{part.text}
			</mark>{:else}{part.text}{/if}{/each}
{/if}
