import { Component, DestroyRef, computed, inject, signal } from '@angular/core';
import { HttpErrorResponse } from '@angular/common/http';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { timeout } from 'rxjs';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { AuthService, apiError } from './auth.service';
import { ActivityService } from './api/services/activity.service';
import { ActivityEvent } from './api/models';

const labels: Record<string, string> = { 'roster.created': 'Draft roster created', 'roster.published': 'Roster published', 'roster.archived': 'Roster archived', 'shift.created': 'Shift added', 'shift.updated': 'Shift updated', 'shift.assigned': 'Employee assigned', 'shift.unassigned': 'Assignment removed', 'shift.deleted': 'Shift removed', 'employee.created': 'Team member added', 'employee.updated': 'Team member updated', 'location.created': 'Location added', 'location.updated': 'Location updated', 'job_role.created': 'Job role added', 'job_role.updated': 'Job role updated' };
@Component({ selector: 'app-activity', imports: [RouterLink], templateUrl: './activity.html' })
export class Activity {
  readonly auth = inject(AuthService);
  private readonly api = inject(ActivityService);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly destroyRef = inject(DestroyRef);
  readonly memberships = computed(() => (this.auth.identity()?.memberships ?? []).filter(org => ['admin', 'manager'].includes(org.role)));
  readonly selected = signal(this.memberships().find(org => org.org_id === this.route.snapshot.queryParamMap.get('org'))?.org_id ?? this.memberships()[0]?.org_id ?? '');
  readonly membership = computed(() => this.memberships().find(org => org.org_id === this.selected()));
  readonly items = signal<ActivityEvent[]>([]);
  readonly error = signal('');
  readonly loading = signal(false);
  readonly offset = signal(0);
  readonly hasMore = signal(false);
  private version = 0;
  constructor() { this.load(); }
  changeOrganization(event: Event): void { this.selected.set((event.target as HTMLSelectElement).value); this.offset.set(0); void this.router.navigate([], { relativeTo: this.route, queryParams: { org: this.selected() }, replaceUrl: true }); this.load(); }
  paginate(direction: number): void { this.offset.update(offset => Math.max(0, offset + direction * 25)); this.load(); }
  load(): void {
    const version = ++this.version; this.items.set([]); this.error.set(''); this.hasMore.set(false); this.loading.set(true);
    this.api.listActivity({ orgID: this.selected(), limit: 25, offset: this.offset() }).pipe(timeout(15000), takeUntilDestroyed(this.destroyRef)).subscribe({
      next: page => { if (version !== this.version) return; this.items.set(page.items); this.hasMore.set(page.has_more); this.loading.set(false); },
      error: error => { if (version !== this.version) return; this.error.set(apiError(error)); this.loading.set(false); if (error instanceof HttpErrorResponse && error.status === 401) { this.auth.identity.set(null); void this.router.navigateByUrl('/login'); } },
    });
  }
  label(action: string): string { return labels[action] ?? 'Workspace updated'; }
  display(instant: string): string { return new Intl.DateTimeFormat('en-GB', { timeZone: this.membership()?.timezone ?? 'Africa/Lagos', dateStyle: 'medium', timeStyle: 'short' }).format(new Date(instant)); }
}
