import type { DirectoryTask } from '#shared/api';

interface Segment {
	readonly text: string;
	readonly hit: boolean;
}

export const needleOf = (query: string): string => query.trim().toLowerCase();

export const matches = (task: DirectoryTask, needle: string): boolean =>
	!needle ||
	[task.name, task.description, task.project.slug, task.project.name, ...task.assignees]
		.join(' ')
		.toLowerCase()
		.includes(needle);

export const highlight = (text: string, needle: string): Segment[] => {
	const lower = text.toLowerCase();
	const segments: Segment[] = [];
	let from = 0;
	for (let at = needle ? lower.indexOf(needle) : -1; at >= 0; at = lower.indexOf(needle, from)) {
		if (at > from) segments.push({ text: text.slice(from, at), hit: false });
		from = at + needle.length;
		segments.push({ text: text.slice(at, from), hit: true });
	}
	if (from < text.length || !segments.length) segments.push({ text: text.slice(from), hit: false });
	return segments;
};
