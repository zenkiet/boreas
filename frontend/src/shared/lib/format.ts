const MINUTE = 60_000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

const STAMP = new Intl.DateTimeFormat('en-US', {
	month: 'short',
	day: 'numeric',
	hour: 'numeric',
	minute: '2-digit'
});

const elapsed = (iso: string, now: number): number => Math.max(0, now - Date.parse(iso));

export const plural = (count: number, noun: string, many = `${noun}s`): string =>
	`${count} ${count === 1 ? noun : many}`;

export const ago = (iso: string, now: number): string => {
	const ms = elapsed(iso, now);
	if (ms < HOUR) return `${Math.max(1, Math.floor(ms / MINUTE))}m`;
	if (ms < DAY) return `${Math.floor(ms / HOUR)}h`;
	return `${Math.floor(ms / DAY)}d`;
};

export const agoLong = (iso: string, now: number): string => {
	const ms = elapsed(iso, now);
	if (ms < MINUTE) return 'just now';
	if (ms < HOUR) return `${Math.floor(ms / MINUTE)} min ago`;
	if (ms < DAY) return `${plural(Math.floor(ms / HOUR), 'hour')} ago`;
	return `${plural(Math.floor(ms / DAY), 'day')} ago`;
};

export const stamp = (iso: string): string => STAMP.format(Date.parse(iso));
