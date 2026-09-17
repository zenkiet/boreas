import { browser } from '$app/environment';

const KEY = 'boreas-theme';
const MODES = ['system', 'light', 'dark'] as const;

export type ThemeMode = (typeof MODES)[number];

const stored = browser ? localStorage.getItem(KEY) : null;

class Theme {
	mode = $state<ThemeMode>(stored === 'light' || stored === 'dark' ? stored : 'system');
	prefersDark = $state(false);

	get resolved(): 'light' | 'dark' {
		return this.mode === 'system' ? (this.prefersDark ? 'dark' : 'light') : this.mode;
	}

	cycle() {
		this.mode = MODES[(MODES.indexOf(this.mode) + 1) % MODES.length];
		if (this.mode === 'system') localStorage.removeItem(KEY);
		else localStorage.setItem(KEY, this.mode);
	}
}

export const theme = new Theme();
