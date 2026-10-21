<script lang="ts">
	import { marked } from 'marked';
	import policy from './privacy.md?raw';

	const html = marked.parse(policy, { async: false });
</script>

<svelte:head>
	<title>Privacy Policy · Boreas</title>
</svelte:head>

<article class="doc mx-auto w-full max-w-160 px-5 py-8">
	<!-- eslint-disable-next-line svelte/no-at-html-tags -->
	{@html html}
</article>

<style>
	.doc {
		line-height: 1.65;
		color: var(--tui-text-secondary);

		:global {
			h1 {
				margin: 0 0 0.5rem;
				font-size: 1.75rem;
				font-weight: 600;
				letter-spacing: -0.02em;
				color: var(--tui-text-primary);
			}

			h2 {
				margin: 2rem 0 0.5rem;
				font-size: 1.125rem;
				font-weight: 600;
				color: var(--tui-text-primary);
			}

			p,
			ul,
			table {
				margin: 0 0 1rem;
			}

			ul {
				padding-inline-start: 1.25rem;
				list-style: disc;
			}

			strong {
				font-weight: 600;
				color: var(--tui-text-primary);
			}

			a {
				color: var(--tui-text-action);
				text-decoration: underline;
				text-underline-offset: 2px;
				overflow-wrap: anywhere;
			}

			code {
				padding: 1px 5px;
				border-radius: 5px;
				background: var(--tui-background-neutral-2);
				font-family: var(--app-font-mono);
				font-size: 0.8125rem;
				color: var(--tui-text-primary);
			}

			table {
				display: block;
				overflow-x: auto;
				border-collapse: collapse;
				font-size: 0.875rem;
			}

			th,
			td {
				padding: 0.5rem 0.75rem;
				border: 1px solid var(--tui-border-normal);
				text-align: start;
				vertical-align: top;
			}

			th {
				font-weight: 600;
				color: var(--tui-text-primary);
			}
		}
	}
</style>
