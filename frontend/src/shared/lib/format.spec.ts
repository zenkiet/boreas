import assert from 'node:assert/strict';
import { test } from 'node:test';

process.env.TZ = 'UTC';
const { ago, agoLong, plural, stamp } = await import('./format.ts');

const NOW = Date.parse('2026-11-07T14:30:00Z');
const before = (minutes: number) => new Date(NOW - minutes * 60_000).toISOString();

test('ago floors to minutes, hours and days, never under a minute', () => {
	assert.equal(ago(before(0), NOW), '1m');
	assert.equal(ago(before(-5), NOW), '1m');
	assert.equal(ago(before(3), NOW), '3m');
	assert.equal(ago(before(125), NOW), '2h');
	assert.equal(ago(before(5 * 1440 + 30), NOW), '5d');
});

test('agoLong reads as a phrase', () => {
	assert.equal(agoLong(before(0), NOW), 'just now');
	assert.equal(agoLong(before(14), NOW), '14 min ago');
	assert.equal(agoLong(before(60), NOW), '1 hour ago');
	assert.equal(agoLong(before(130), NOW), '2 hours ago');
	assert.equal(agoLong(before(1440), NOW), '1 day ago');
	assert.equal(agoLong(before(14400), NOW), '10 days ago');
});

test('stamp gives the short date and time', () => {
	assert.equal(stamp('2026-11-07T14:27:00Z').replace(/\s/g, ' '), 'Nov 7, 2:27 PM');
});

test('plural picks the noun form', () => {
	assert.equal(plural(1, 'task'), '1 task');
	assert.equal(plural(3, 'task'), '3 tasks');
	assert.equal(plural(2, 'person', 'people'), '2 people');
});
