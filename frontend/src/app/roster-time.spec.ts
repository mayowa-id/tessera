import { localInstant, timeCandidates, wallTime } from './roster-time';

describe('Organization-local shift times', () => {
  it('converts Lagos local time without using the browser timezone', () => {
    expect(localInstant('2026-11-02T09:00', 'Africa/Lagos', 'earlier')).toBe('2026-11-02T08:00:00.000Z');
    expect(wallTime('2026-11-02T08:00:00Z', 'Africa/Lagos')).toBe('2026-11-02T09:00');
  });
  it('rejects skipped DST minutes and invalid calendar dates', () => {
    expect(timeCandidates('2026-03-08T02:30', 'America/New_York')).toEqual([]);
    expect(timeCandidates('2026-02-30T10:00', 'Africa/Lagos')).toEqual([]);
  });
  it('lets the user choose either occurrence of a repeated minute', () => {
    expect(timeCandidates('2026-11-01T01:30', 'America/New_York')).toEqual(['2026-11-01T05:30:00.000Z', '2026-11-01T06:30:00.000Z']);
    expect(localInstant('2026-11-01T01:30', 'America/New_York', 'later')).toBe('2026-11-01T06:30:00.000Z');
  });
});
