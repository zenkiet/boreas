import hljs from 'highlight.js/lib/core';
import bash from 'highlight.js/lib/languages/bash';
import css from 'highlight.js/lib/languages/css';
import diff from 'highlight.js/lib/languages/diff';
import go from 'highlight.js/lib/languages/go';
import javascript from 'highlight.js/lib/languages/javascript';
import json from 'highlight.js/lib/languages/json';
import sql from 'highlight.js/lib/languages/sql';
import typescript from 'highlight.js/lib/languages/typescript';
import xml from 'highlight.js/lib/languages/xml';
import yaml from 'highlight.js/lib/languages/yaml';
import { Marked } from 'marked';

for (const [name, language] of Object.entries({
	bash,
	css,
	diff,
	go,
	javascript,
	json,
	sql,
	typescript,
	xml,
	yaml
}))
	hljs.registerLanguage(name, language);

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

let renderer: Marked | undefined;

const createRenderer = () =>
	new Marked({
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
			checkbox: ({ checked }) =>
				`<input type="checkbox" disabled${checked ? ' checked' : ''} aria-label="Task"> `,
			code({ text, lang }) {
				const language = lang?.split(/\s+/)[0] ?? '';
				const known = hljs.getLanguage(language);
				const code = known ? hljs.highlight(text, { language }).value : escapeHtml(text);
				const tag = known ? `<code class="language-${escapeHtml(language)}">` : '<code>';
				return `<div class="code-block"><pre>${tag}${code}</code></pre></div>`;
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

export const NOTE_SANITIZER: SanitizerConfig = {
	elements:
		'a blockquote br code del div em h1 h2 h3 h4 h5 h6 hr input li ol p pre span strong table tbody td th thead tr ul'.split(
			' '
		),
	attributes: 'align aria-label checked class disabled href rel start target type'.split(' ')
};

export const codeName = (pre: Element): string => {
	const key = /language-([\w-]+)/.exec(pre.querySelector('code')?.className ?? '')?.[1];
	return (key && hljs.getLanguage(key)?.name) || '';
};

export const renderNote = (markdown: string): string =>
	(renderer ??= createRenderer()).parse(markdown.replace(/^([ \t]*)\\`\\`\\`/gm, '$1```'), {
		async: false
	}) as string;
