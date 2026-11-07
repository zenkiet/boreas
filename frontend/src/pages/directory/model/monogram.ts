export const monogram = (name: string): string => {
	const words = name.split(/\s+/).filter(Boolean);
	const picked = words.length > 1 ? [words[0], words[words.length - 1]] : words;
	return picked
		.map((word) => word[0])
		.join('')
		.toUpperCase();
};
