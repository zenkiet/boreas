import { on } from 'svelte/events';
import { MediaQuery } from 'svelte/reactivity';
import { browser } from '$app/env';

type Mode = 'light' | 'dark';

const KEY = 'boreas-theme';

const read = (): Mode | null => {
	try {
		const stored = localStorage.getItem(KEY);
		return stored === 'light' || stored === 'dark' ? stored : null;
	} catch {
		return null;
	}
};

class Theme {
	#chosen = $state<Mode | null>(browser ? read() : null);
	#prefersDark = new MediaQuery('(prefers-color-scheme: dark)');

	constructor() {
		if (browser) on(window, 'storage', () => (this.#chosen = read()));
	}

	get chosen(): Mode | null {
		return this.#chosen;
	}

	get resolved(): Mode {
		return this.#chosen ?? (this.#prefersDark.current ? 'dark' : 'light');
	}

	toggle() {
		const next: Mode = this.resolved === 'dark' ? 'light' : 'dark';
		const system: Mode = this.#prefersDark.current ? 'dark' : 'light';
		this.#chosen = next === system ? null : next;
		if (this.#chosen) localStorage.setItem(KEY, next);
		else localStorage.removeItem(KEY);
	}
}

export const theme = new Theme();
