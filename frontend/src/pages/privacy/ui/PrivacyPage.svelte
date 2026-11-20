<script lang="ts">
	import { Marked, Renderer } from 'marked';
	import type { Attachment } from 'svelte/attachments';
	import { on } from 'svelte/events';
	import { PageTop } from '#shared/ui';
	import './policy.css';
	import policy from './privacy.md?raw';

	const anchor = (heading: string) => {
		const label = heading.replace(/^\d+\.\s*|\s*\(.*\)$/g, '');
		return { label, id: label.toLowerCase().replace(/[^a-z0-9]+/g, '-') };
	};

	let titled = '';

	const marked = new Marked({
		renderer: {
			heading({ tokens, depth, text }) {
				titled = `${anchor(text).id}-title`;
				return `<h${depth} id="${titled}">${this.parser.parseInline(tokens)}</h${depth}>`;
			},
			table(token) {
				return `<div class="policy-table" tabindex="0" role="region" aria-labelledby="${titled}">${Renderer.prototype.table.call(this, token)}</div>`;
			}
		}
	});
	const html = (markdown: string) => marked.parse(markdown, { async: false });

	const [head, summary, ...sections] = policy.split(/^(?=## )/m);
	const toc = sections.map((section) => anchor(section.slice(3, section.indexOf('\n'))));

	let current = $state(toc[0].id);

	const spy: Attachment<HTMLElement> = (nav) => {
		if (CSS.supports('scroll-target-group: auto')) {
			const sync = () =>
				(current =
					nav.querySelector<HTMLAnchorElement>('a:target-current')?.hash.slice(1) ?? current);
			sync();
			return on(document, 'scrollend', () =>
				requestAnimationFrame(() => requestAnimationFrame(sync))
			);
		}
		const watcher = new IntersectionObserver(
			(entries) => {
				for (const { isIntersecting, target } of entries) if (isIntersecting) current = target.id;
			},
			{ rootMargin: '0px 0px -90% 0px' }
		);
		for (const { id } of toc) watcher.observe(document.getElementById(id)!);
		return () => watcher.disconnect();
	};
</script>

<svelte:head><title>Privacy Policy · Boreas</title></svelte:head>

<PageTop />
<main class="grid lg:grid-cols-[minmax(0,720px)_220px] lg:justify-center lg:gap-16 lg:px-6">
	<article class="policy mx-auto w-full max-w-180 px-6 pt-12 pb-20">
		<span class="font-mono text-caption tracking-[0.12em] text-accent-text uppercase">Legal</span>
		<div class="policy-head">{@html html(head)}</div>
		<section class="policy-summary mb-2 rounded-box bg-surface px-5 py-[18px] shadow-1">
			{@html html(summary)}
		</section>
		{#each sections as section, i (toc[i].id)}
			<section id={toc[i].id} tabindex="-1">{@html html(section)}</section>
		{/each}
	</article>
	<nav
		{@attach spy}
		aria-label="On this page"
		class="policy-toc sticky top-6 mt-12 self-start border-s border-sep ps-4 max-lg:hidden"
	>
		<p class="mb-2 text-caption text-label-3" aria-hidden="true">On this page</p>
		{#each toc as { id, label } (id)}
			<a
				href="#{id}"
				aria-current={id === current}
				class="block py-[5px] text-footnote text-label-2 no-underline hover:text-label"
			>
				{label}
			</a>
		{/each}
	</nav>
</main>
<footer
	class="flex flex-wrap justify-center gap-x-4 gap-y-1 px-5 pt-2 pb-10 text-footnote text-label-3"
>
	<span>© 2026 ZenSoftware. All rights reserved.</span>
</footer>
