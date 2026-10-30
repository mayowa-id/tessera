import { Component, DestroyRef, computed, inject, signal } from '@angular/core';
import { HttpErrorResponse } from '@angular/common/http';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { timeout } from 'rxjs';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { AuthService, apiError } from './auth.service';
import { SchedulesService } from './api/services/schedules.service';
import { MyShift } from './api/models';
import { localInstant, wallTime } from './roster-time';

function addDays(day: string, days: number): string { const date = new Date(day + 'T12:00:00Z'); date.setUTCDate(date.getUTCDate() + days); return date.toISOString().slice(0, 10); }
@Component({ selector: 'app-my-shifts', imports: [RouterLink], templateUrl: './my-shifts.html' })
export class MyShifts {
  readonly auth = inject(AuthService);
  private readonly api = inject(SchedulesService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);
  private readonly destroyRef = inject(DestroyRef);
  readonly memberships = computed(() => this.auth.identity()?.memberships ?? []);
  readonly selected = signal(this.memberships().find(org => org.org_id === this.route.snapshot.queryParamMap.get('org'))?.org_id ?? this.memberships()[0]?.org_id ?? '');
  readonly membership = computed(() => this.memberships().find(org => org.org_id === this.selected()));
  readonly isManager = computed(() => ['admin', 'manager'].includes(this.membership()?.role ?? ''));
  readonly fromDate = signal(wallTime(new Date(), this.membership()?.timezone ?? 'Africa/Lagos').slice(0, 10));
  readonly toDate = signal(addDays(this.fromDate(), 28));
  readonly items = signal<MyShift[]>([]);
  readonly loading = signal(false);
  readonly error = signal('');
  readonly offset = signal(0);
  readonly hasMore = signal(false);
  private version = 0;
  constructor() { this.load(); }
  changeOrganization(event: Event): void { this.selected.set((event.target as HTMLSelectElement).value); this.offset.set(0); void this.router.navigate([], { relativeTo: this.route, queryParams: { org: this.selected() }, replaceUrl: true }); this.load(); }
  filter(event: Event, from: string, to: string): void { event.preventDefault(); this.fromDate.set(from); this.toDate.set(to); this.offset.set(0); this.load(); }
  paginate(direction: number): void { this.offset.update(offset => Math.max(0, offset + direction * 25)); this.load(); }
  load(): void {
    const version = ++this.version; this.error.set(''); this.items.set([]); this.hasMore.set(false);
    if (!this.membership()) { this.loading.set(false); return; }
    let from: string, to: string;
    try { from = localInstant(this.fromDate() + 'T00:00', this.membership()!.timezone, 'earlier'); to = localInstant(this.toDate() + 'T00:00', this.membership()!.timezone, 'later'); } catch { this.loading.set(false); this.error.set('Choose valid start and end dates.'); return; }
    if (Date.parse(to) <= Date.parse(from) || Date.parse(to) - Date.parse(from) > 62 * 86400000) { this.loading.set(false); this.error.set('Choose a date range of more than zero and at most 62 elapsed days.'); return; }
    this.loading.set(true);
    this.api.getMyShifts({ orgID: this.selected(), from, to, limit: 25, offset: this.offset() }).pipe(timeout(15000), takeUntilDestroyed(this.destroyRef)).subscribe({
      next: page => { if (version !== this.version) return; this.items.set(page.items); this.hasMore.set(page.has_more); this.loading.set(false); },
      error: error => { if (version !== this.version) return; this.loading.set(false); this.error.set(apiError(error)); if (error instanceof HttpErrorResponse && error.status === 401) { this.auth.identity.set(null); void this.router.navigateByUrl('/login'); } },
    });
  }
  display(value: string, zone: string): string { return new Intl.DateTimeFormat('en-GB', { timeZone: zone, weekday: 'short', day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).format(new Date(value)); }
}
