export const copyText = (text: string): Promise<boolean> =>
	navigator.clipboard?.writeText(text).then(
		() => true,
		() => false
	) ?? Promise.resolve(false);
