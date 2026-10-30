/** Format a wall-clock minute in an explicit IANA zone, independent of the browser zone. */
export function wallTime(instant: string | Date, zone: string): string {
  const parts = new Intl.DateTimeFormat('en-CA', { timeZone: zone, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).formatToParts(new Date(instant));
  const value = (type: string) => parts.find(part => part.type === type)!.value;
  return `${value('year')}-${value('month')}-${value('day')}T${value('hour')}:${value('minute')}`;
}

/** A repeated DST minute yields two instants; a skipped clock minute yields none. */
export function timeCandidates(value: string, zone: string): string[] {
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(value)) return [];
  const guess = new Date(value + ':00Z').getTime();
  if (!Number.isFinite(guess) || new Date(guess).toISOString().slice(0, 16) !== value) return [];
  const offsets = new Set<number>();
  for (const hours of [-36, -12, 0, 12, 36]) {
    const sample = guess + hours * 3600000;
    const local = Date.parse(wallTime(new Date(sample), zone) + ':00Z');
    offsets.add(local - sample);
  }
  return [...offsets].map(offset => new Date(guess - offset).toISOString()).filter(candidate => wallTime(candidate, zone) === value).sort();
}
export function localInstant(value: string, zone: string, occurrence: string): string {
  const candidates = timeCandidates(value, zone);
  if (!candidates.length) throw new Error('This local time does not exist or is invalid. Check the date and clock-change time.');
  return occurrence === 'later' ? candidates[candidates.length - 1] : candidates[0];
}
export function localMonday(zone: string): string {
  const date = new Date(wallTime(new Date(), zone).slice(0, 10) + 'T12:00:00Z');
  date.setUTCDate(date.getUTCDate() - (date.getUTCDay() + 6) % 7);
  return date.toISOString().slice(0, 10);
}
