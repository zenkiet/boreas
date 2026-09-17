const DATE_TIME = new Intl.DateTimeFormat('en-US', {
	month: 'short',
	day: 'numeric',
	year: 'numeric',
	hour: 'numeric',
	minute: '2-digit'
});

export const formatDateTime = (iso: string): string => DATE_TIME.format(new Date(iso));
