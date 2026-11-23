const MINUTE = 60_000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

const STAMP = new Intl.DateTimeFormat('en-US', {
	month: 'short',
	day: 'numeric',
	hour: 'numeric',
	minute: '2-digit'
});

const NARROW =
	'DurationFormat' in Intl ? new Intl.DurationFormat('en-US', { style: 'narrow' }) : undefined;

const elapsed = (iso: string, now: number): number => Math.max(0, now - Date.parse(iso));

const narrow = (span: { minutes: number } | { hours: number } | { days: number }, text: string) =>
	NARROW?.format(span) ?? text;

export const byInstant = (a: string, b: string): number =>
	typeof Temporal === 'object' ? Temporal.Instant.compare(a, b) : Date.parse(a) - Date.parse(b);

export const byName = new Intl.Collator('en', { numeric: true }).compare;

export const plural = (count: number, noun: string, many = `${noun}s`): string =>
	`${count} ${count === 1 ? noun : many}`;

export const ago = (iso: string, now: number): string => {
	const ms = elapsed(iso, now);
	if (ms < HOUR) {
		const minutes = Math.max(1, Math.floor(ms / MINUTE));
		return narrow({ minutes }, `${minutes}m`);
	}
	if (ms < DAY) {
		const hours = Math.floor(ms / HOUR);
		return narrow({ hours }, `${hours}h`);
	}
	const days = Math.floor(ms / DAY);
	return narrow({ days }, `${days}d`);
};

export const agoLong = (iso: string, now: number): string => {
	const ms = elapsed(iso, now);
	if (ms < MINUTE) return 'just now';
	if (ms < HOUR) return `${Math.floor(ms / MINUTE)} min ago`;
	if (ms < DAY) return `${plural(Math.floor(ms / HOUR), 'hour')} ago`;
	return `${plural(Math.floor(ms / DAY), 'day')} ago`;
};

export const stamp = (iso: string): string => STAMP.format(Date.parse(iso));
