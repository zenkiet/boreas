<script lang="ts">
	import { Marked, Renderer } from 'marked';
	import { PageTop } from '#shared/ui';
	import './policy.css';
	import policy from './privacy.md?raw';

	const anchor = (heading: string) => {
		const label = heading.replace(/^\d+\.\s*|\s*\(.*\)$/g, '');
		return { label, id: label.toLowerCase().replace(/[^a-z0-9]+/g, '-') };
	};

	const marked = new Marked({
		renderer: {
			heading({ tokens, depth, text }) {
				return `<h${depth} id="${anchor(text).id}">${this.parser.parseInline(tokens)}</h${depth}>`;
			},
			table(token) {
				return `<div class="policy-table">${Renderer.prototype.table.call(this, token)}</div>`;
			}
		}
	});
	const html = (markdown: string) => marked.parse(markdown, { async: false });

	const [head, summary, ...sections] = policy.split(/^(?=## )/m);
	const toc = sections.map((section) => anchor(section.slice(3, section.indexOf('\n'))));

	let current = $state(toc[0].id);

	const spy = () => {
		const passed = toc.findLast(
			({ id }) => document.getElementById(id)!.getBoundingClientRect().top < innerHeight / 3
		);
		current = (passed ?? toc[0]).id;
	};
</script>

<svelte:head><title>Privacy Policy · Boreas</title></svelte:head>

<svelte:window onscroll={spy} />

<PageTop />
<div class="grid lg:grid-cols-[minmax(0,720px)_220px] lg:justify-center lg:gap-16 lg:px-6">
	<article class="policy mx-auto w-full max-w-180 px-6 pt-12 pb-20">
		<span class="font-mono text-caption tracking-[0.12em] text-accent-text uppercase">Legal</span>
		<div class="policy-head">{@html html(head)}</div>
		<section class="policy-summary mb-2 rounded-box bg-surface px-5 py-[18px] shadow-1">
			{@html html(summary)}
		</section>
		{@html html(sections.join(''))}
	</article>
	<nav
		aria-label="On this page"
		class="sticky top-6 mt-12 self-start border-l border-sep pl-4 max-lg:hidden"
	>
		<p class="mb-2 text-caption text-label-3" aria-hidden="true">On this page</p>
		{#each toc as { id, label } (id)}
			<a
				href="#{id}"
				aria-current={id === current ? 'location' : undefined}
				class="block py-[5px] text-footnote text-label-2 no-underline hover:text-label aria-[current=location]:font-semibold aria-[current=location]:text-accent-text"
			>
				{label}
			</a>
		{/each}
	</nav>
</div>
<footer
	class="flex flex-wrap justify-center gap-x-4 gap-y-1 px-5 pt-2 pb-10 text-footnote text-label-3"
>
	<span>© 2026 ZenSoftware. All rights reserved.</span>
</footer>
