<script lang="ts">
	import { mount, unmount } from 'svelte';
	import type { Attachment } from 'svelte/attachments';
	import { CopyButton } from '#shared/ui';
	import { codeName, NOTE_SANITIZER, renderNote } from '../lib/render-note.ts';
	import './note-markdown.css';

	let { source }: { source: string } = $props();

	const note =
		(markdown: string): Attachment<HTMLElement> =>
		(root) => {
			const html = renderNote(markdown);
			if ('setHTML' in Element.prototype) root.setHTML(html, { sanitizer: NOTE_SANITIZER });
			else root.innerHTML = html;
			const buttons = [...root.querySelectorAll('pre')].map((pre) => {
				const name = codeName(pre);
				pre.tabIndex = 0;
				pre.setAttribute('role', 'region');
				pre.setAttribute('aria-label', name ? `${name} code` : 'Code');
				return mount(CopyButton, {
					target: pre.parentElement!,
					props: {
						text: pre.textContent ?? '',
						label: name ? `Copy ${name} code` : 'Copy code',
						compact: true
					}
				});
			});
			return () => buttons.forEach((button) => unmount(button));
		};
</script>

<div class="markdown" {@attach note(source)}></div>
