const graphemes = new Intl.Segmenter();

const first = (word: string): string => [...graphemes.segment(word)][0].segment;

export const initials = (username: string): string => {
	const words = username.split(/[\s._-]+/).filter(Boolean);
	const picked = words.length > 1 ? [words[0], words[words.length - 1]] : words;
	return picked.map(first).join('').toUpperCase();
};

export const toneOf = (username: string): number => {
	let hash = 0x811c9dc5;
	for (let i = 0; i < username.length; i++) {
		hash = Math.imul(hash ^ username.charCodeAt(i), 0x01000193);
	}
	return (hash >>> 0) % 7;
};
