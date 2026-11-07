const copyBySelection = (text: string): boolean => {
	const focused = document.activeElement as HTMLElement | null;
	const input = document.createElement('textarea');
	input.value = text;
	input.style.position = 'fixed';
	input.style.opacity = '0';
	document.body.append(input);
	input.select();
	try {
		return document.execCommand('copy');
	} catch {
		return false;
	} finally {
		input.remove();
		focused?.focus();
	}
};

export const copyText = (text: string): Promise<boolean> =>
	navigator.clipboard
		? navigator.clipboard.writeText(text).then(
				() => true,
				() => copyBySelection(text)
			)
		: Promise.resolve(copyBySelection(text));
