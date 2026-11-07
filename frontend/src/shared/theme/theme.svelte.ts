import { MediaQuery } from 'svelte/reactivity';
import { browser } from '$app/env';

type Mode = 'light' | 'dark';

const KEY = 'boreas-theme';

const stored = browser ? localStorage.getItem(KEY) : null;

class Theme {
	#chosen = $state<Mode | null>(stored === 'light' || stored === 'dark' ? stored : null);
	#prefersDark = new MediaQuery('(prefers-color-scheme: dark)');

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
