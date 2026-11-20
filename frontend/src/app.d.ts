declare global {
	interface Element {
		setHTML(html: string, options?: { sanitizer?: SanitizerConfig }): void;
	}

	interface ScrollIntoViewOptions {
		container?: 'all' | 'nearest';
	}
}

declare module 'svelte/elements' {
	// eslint-disable-next-line @typescript-eslint/no-unused-vars
	interface HTMLAttributes<T extends EventTarget> {
		focusgroup?: string;
	}
}

export {};
