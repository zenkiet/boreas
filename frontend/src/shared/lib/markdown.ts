import hljs from 'highlight.js';
import { Marked } from 'marked';

const BLOCKED_SCHEME = /^(?:javascript|data|vbscript|blob):/i;

// eslint-disable-next-line no-control-regex
const CONTROL_CHARS = /[\u0000-\u0020]/g;

const HTTP_SCHEME = /^https?:/i;

const escapeHtml = (value: string) =>
	value
		.replaceAll('&', '&amp;')
		.replaceAll('<', '&lt;')
		.replaceAll('>', '&gt;')
		.replaceAll('"', '&quot;');

const renderer = new Marked({
	breaks: true,
	gfm: true,
	extensions: [
		{
			name: 'deepLink',
			level: 'inline',
			start: (src) => src.search(/[a-z][a-z\d+.-]*:\/\//i),
			tokenizer(src, tokens) {
				if (this.lexer.state.inLink || /\w/.test(tokens.at(-1)?.raw.slice(-1) ?? '')) return;
				const url = /^[a-z][a-z\d+.-]*:\/\/[^\s<>"']*/i
					.exec(src)?.[0]
					.replace(/[.,!?;:)\]}]+$/, '');
				if (!url || /^(?:https?|ftp):/i.test(url) || BLOCKED_SCHEME.test(url)) return;
				return {
					type: 'link',
					raw: url,
					href: url,
					text: url,
					tokens: [{ type: 'text', raw: url, text: url }]
				};
			}
		}
	],
	renderer: {
		html: () => '',
		image: () => '',
		code({ text, lang }) {
			const language = lang?.split(/\s+/)[0] ?? '';
			const code = hljs.getLanguage(language)
				? hljs.highlight(text, { language }).value
				: escapeHtml(text);
			return `<div class="code-block">
				<button type="button" data-copy-code aria-label="Copy code" title="Copy code" aria-live="polite">
					<svg data-copy-icon width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
						<rect x="8" y="8" width="13" height="13" rx="2"/>
						<path d="M16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3"/>
					</svg>
					<svg data-copy-check width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
						<path d="m5 12 4 4L19 6"/>
					</svg>
				</button>
				<pre><code>${code}</code></pre>
			</div>`;
		},
		link({ href, tokens }) {
			const text = this.parser.parseInline(tokens);
			const normalizedHref = href.replace(CONTROL_CHARS, '');

			if (BLOCKED_SCHEME.test(normalizedHref)) return text;

			const isWebLink = HTTP_SCHEME.test(normalizedHref);
			const target = isWebLink ? ' target="_blank"' : '';
			const rel = isWebLink ? ' rel="noopener nofollow"' : '';

			return `<a href="${escapeHtml(href)}"${target}${rel}>${text}</a>`;
		}
	}
});

export const renderNote = (markdown: string): string =>
	renderer.parse(markdown.replace(/^([ \t]*)\\`\\`\\`/gm, '$1```'), { async: false }) as string;
