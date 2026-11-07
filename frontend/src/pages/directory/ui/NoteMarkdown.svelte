<script lang="ts">
	import type { Attachment } from 'svelte/attachments';
	import { on } from 'svelte/events';
	import { copyText } from '#shared/lib';
	import { renderNote } from '../lib/render-note.ts';
	import './note-markdown.css';

	let { source }: { source: string } = $props();

	const show = (button: HTMLButtonElement, label: string, state?: string) => {
		button.title = label;
		button.setAttribute('aria-label', label);
		if (state) button.dataset.copyState = state;
		else delete button.dataset.copyState;
	};

	const copyCode: Attachment<HTMLElement> = (node) => {
		const resets: ReturnType<typeof setTimeout>[] = [];
		const off = on(node, 'click', (event) => {
			const button = (event.target as Element).closest<HTMLButtonElement>('[data-copy-code]');
			const code = button?.parentElement?.querySelector('code');
			if (!button || !code) return;
			copyText(code.textContent ?? '').then((copied) => {
				show(button, copied ? 'Copied' : 'Copy failed', copied ? 'copied' : 'failed');
				resets.push(setTimeout(() => show(button, 'Copy code'), 2000));
			});
		});
		return () => {
			off();
			resets.forEach((reset) => clearTimeout(reset));
		};
	};
</script>

<div class="markdown" {@attach copyCode}>
	{@html renderNote(source)}
</div>
