import { Component, DestroyRef, computed, inject, signal } from '@angular/core';
import { HttpErrorResponse } from '@angular/common/http';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { EMPTY, Observable, expand, finalize, forkJoin, reduce, timeout } from 'rxjs';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { AuthService, apiError } from './auth.service';
import { RostersService } from './api/services/rosters.service';
import { StaffService } from './api/services/staff.service';
import { CatalogItem, Employee, Roster, RosterDetail, ScheduleViolation, Shift, ShiftResult } from './api/models';
import { localInstant, localMonday, timeCandidates, wallTime } from './roster-time';

function collect<T>(load: (offset: number) => Observable<{ items: T[]; offset: number; limit: number; has_more: boolean }>): Observable<T[]> {
  return load(0).pipe(expand(page => page.has_more ? load(page.offset + page.limit) : EMPTY), reduce((items, page) => items.concat(page.items), [] as T[]));
}

@Component({ selector: 'app-rosters', imports: [ReactiveFormsModule, RouterLink], templateUrl: './rosters.html' })
export class Rosters {
  readonly auth = inject(AuthService);
  private readonly api = inject(RostersService);
  private readonly staff = inject(StaffService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);
  private readonly destroyRef = inject(DestroyRef);
  readonly memberships = computed(() => (this.auth.identity()?.memberships ?? []).filter(org => ['admin', 'manager'].includes(org.role)));
  readonly selected = signal(this.memberships().find(org => org.org_id === this.route.snapshot.queryParamMap.get('org'))?.org_id ?? this.memberships()[0]?.org_id ?? '');
  readonly membership = computed(() => this.memberships().find(org => org.org_id === this.selected()));
  readonly zone = computed(() => this.detail()?.roster.timezone ?? this.membership()?.timezone ?? 'Africa/Lagos');
  readonly week = signal(localMonday(this.membership()?.timezone ?? 'Africa/Lagos'));
  readonly locations = signal<CatalogItem[]>([]);
  readonly roles = signal<CatalogItem[]>([]);
  readonly employees = signal<Employee[]>([]);
  readonly location = signal('');
  readonly lookupsReady = signal(false);
  readonly lookupError = signal('');
  readonly rosters = signal<Roster[]>([]);
  readonly detail = signal<RosterDetail | null>(null);
  readonly busy = signal(false);
  readonly loading = signal(false);
  readonly detailLoading = signal(false);
  readonly offset = signal(0);
  readonly hasMore = signal(false);
  readonly error = signal('');
  readonly notice = signal('');
  readonly violations = signal<ScheduleViolation[]>([]);
  readonly editorOpen = signal(false);
  readonly editing = signal<Shift | null>(null);
  readonly deleting = signal<Shift | null>(null);
  readonly lifecycleAction = signal<'publish' | 'archive' | null>(null);
  readonly readyToPublish = computed(() => this.detail()?.roster.state === 'draft' && !!this.detail()?.shifts.length && this.detail()!.shifts.every(shift => !!shift.employee_id));
  readonly form = inject(FormBuilder).nonNullable.group({ role: ['', Validators.required], start: ['', Validators.required], end: ['', Validators.required], startOccurrence: ['earlier'], endOccurrence: ['earlier'] });
  private contextVersion = 0;
  private listVersion = 0;
  private detailVersion = 0;

  constructor() { this.loadLookups(); this.loadRosters(); }
  changeOrganization(event: Event): void {
    this.selected.set((event.target as HTMLSelectElement).value);
    this.detail.set(null); this.week.set(localMonday(this.membership()?.timezone ?? 'Africa/Lagos'));
    this.resetContext(); this.loadLookups(); this.loadRosters();
    void this.router.navigate([], { relativeTo: this.route, queryParams: { org: this.selected() }, replaceUrl: true });
  }
  changeWeek(value: string): void { this.week.set(value); this.resetContext(); this.loadRosters(); }
  private resetContext(): void { this.detailVersion++; this.detail.set(null); this.detailLoading.set(false); this.editorOpen.set(false); this.deleting.set(null); this.lifecycleAction.set(null); this.offset.set(0); this.clearFeedback(); }
  private clearFeedback(): void { this.error.set(''); this.notice.set(''); this.violations.set([]); }
  private validWeek(): boolean { const day = new Date(this.week() + 'T12:00:00Z'); return /^\d{4}-\d{2}-\d{2}$/.test(this.week()) && Number.isFinite(day.getTime()) && day.getUTCDay() === 1; }
  loadLookups(): void {
    const version = ++this.contextVersion, orgID = this.selected();
    this.lookupsReady.set(false); this.lookupError.set(''); this.roles.set([]); this.employees.set([]); this.locations.set([]); this.location.set('');
    forkJoin({
      locations: collect(offset => this.staff.listLocations({ orgID, offset, limit: 100 })),
      roles: collect(offset => this.staff.listJobRoles({ orgID, offset, limit: 100, include_inactive: true })),
      employees: collect(offset => this.staff.listEmployees({ orgID, offset, limit: 100, include_inactive: true })),
    }).pipe(timeout(30000), takeUntilDestroyed(this.destroyRef)).subscribe({
      next: data => { if (version !== this.contextVersion) return; this.locations.set(data.locations); this.roles.set(data.roles); this.employees.set(data.employees); this.location.set(data.locations[0]?.id ?? ''); this.lookupsReady.set(true); },
      error: error => { if (version === this.contextVersion) { this.lookupError.set(apiError(error)); this.checkSession(error); } },
    });
  }
  loadRosters(): void {
    const version = ++this.listVersion;
    this.rosters.set([]); this.hasMore.set(false); this.error.set('');
    if (!this.validWeek()) { this.loading.set(false); this.error.set('Choose a Monday to start the roster week.'); return; }
    this.loading.set(true);
    this.api.listRosters({ orgID: this.selected(), week_start: this.week(), limit: 25, offset: this.offset() }).pipe(timeout(15000), takeUntilDestroyed(this.destroyRef)).subscribe({
      next: page => { if (version !== this.listVersion) return; this.rosters.set(page.items); this.hasMore.set(page.has_more); this.loading.set(false); },
      error: error => { if (version === this.listVersion) { this.loading.set(false); this.handleError(error); } },
    });
  }
  paginate(direction: number): void { this.offset.update(offset => Math.max(0, offset + 25 * direction)); this.loadRosters(); }
  createRoster(): void {
    if (this.busy() || !this.location() || !this.lookupsReady() || !this.validWeek()) return;
    this.busy.set(true); this.clearFeedback();
    this.api.createDraftRoster({ orgID: this.selected(), body: { location_id: this.location(), week_start: this.week() } }).pipe(timeout(15000), takeUntilDestroyed(this.destroyRef), finalize(() => this.busy.set(false))).subscribe({
      next: roster => { this.offset.set(0); this.loadRosters(); this.openRoster(roster); this.notice.set('Draft roster created.'); },
      error: error => this.handleError(error),
    });
  }
  openRoster(roster: Roster): void {
    this.clearFeedback();
    this.lifecycleAction.set(null);
    const version = ++this.detailVersion;
    this.detail.set(null); this.editorOpen.set(false); this.deleting.set(null); this.detailLoading.set(true);
    this.api.getRoster({ orgID: this.selected(), rosterID: roster.id }).pipe(timeout(15000), takeUntilDestroyed(this.destroyRef)).subscribe({
      next: detail => { if (version === this.detailVersion) { this.detail.set(detail); this.detailLoading.set(false); } },
      error: error => { if (version === this.detailVersion) { this.detailLoading.set(false); this.handleError(error); } },
    });
  }
  displayTime(value: string): string { return new Intl.DateTimeFormat('en-GB', { timeZone: this.zone(), weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).format(new Date(value)); }
  violationOwner(violation: ScheduleViolation): string { const person = this.employees().find(person => person.id === violation.employee_id); return person ? person.display_name + ': ' : ''; }
  archiveReady(): boolean {
    const detail = this.detail(); if (!detail || detail.roster.state !== 'published') return false;
    const endDate = new Date(detail.roster.week_start + 'T12:00:00Z'); endDate.setUTCDate(endDate.getUTCDate() + 7);
    try { const end = Date.parse(localInstant(endDate.toISOString().slice(0, 10) + 'T00:00', detail.roster.timezone, 'later')); return Date.now() >= Math.max(end, ...detail.shifts.map(shift => Date.parse(shift.ends_at))); } catch { return false; }
  }
  transition(): void {
    const detail = this.detail(), action = this.lifecycleAction(); if (!detail || !action || this.busy()) return;
    this.busy.set(true); this.clearFeedback();
    const params = { orgID: this.selected(), rosterID: detail.roster.id };
    const request = action === 'publish' ? this.api.publishRoster(params) : this.api.archiveRoster(params);
    request.pipe(timeout(15000), takeUntilDestroyed(this.destroyRef), finalize(() => this.busy.set(false))).subscribe({
      next: result => { this.detail.set(result.detail); this.rosters.update(rosters => rosters.map(roster => roster.id === result.detail.roster.id ? result.detail.roster : roster)); this.violations.set(result.violations); this.lifecycleAction.set(null); this.editorOpen.set(false); this.deleting.set(null); this.notice.set(action === 'publish' ? 'Roster published. Employees can now see their assigned shifts.' : 'Roster archived. Published history is retained.'); },
      error: error => this.handleError(error),
    });
  }
  candidates(value: string): string[] { try { return timeCandidates(value, this.zone()); } catch { return []; } }
  eligible(shift: Shift): Employee[] { return this.employees().filter(person => person.id === shift.employee_id || person.active && person.job_role_ids.includes(shift.job_role_id)); }
  openEditor(shift: Shift | null = null): void {
    this.clearFeedback(); this.editing.set(shift); this.deleting.set(null);
    const start = shift ? wallTime(shift.starts_at, this.zone()) : this.week() + 'T09:00';
    const end = shift ? wallTime(shift.ends_at, this.zone()) : this.week() + 'T17:00';
    const occurrence = (wall: string, instant?: string) => instant && this.candidates(wall).length > 1 && Date.parse(this.candidates(wall)[1]) === Date.parse(instant) ? 'later' : 'earlier';
    this.form.reset({ role: shift?.job_role_id ?? this.roles().find(role => role.active)?.id ?? '', start, end, startOccurrence: occurrence(start, shift?.starts_at), endOccurrence: occurrence(end, shift?.ends_at) });
    this.editorOpen.set(true);
  }
  saveShift(): void {
    const detail = this.detail(); if (!detail || this.busy()) return;
    this.clearFeedback(); if (this.form.invalid) { this.form.markAllAsTouched(); this.error.set('Choose a job role and valid start and end times.'); return; }
    const values = this.form.getRawValue();
    let starts_at: string, ends_at: string;
    try { starts_at = localInstant(values.start, this.zone(), values.startOccurrence); ends_at = localInstant(values.end, this.zone(), values.endOccurrence); } catch (error) { this.error.set((error as Error).message); return; }
    if (Date.parse(ends_at) <= Date.parse(starts_at) || Date.parse(ends_at) - Date.parse(starts_at) > 86400000) { this.error.set('A shift must last more than zero and at most 24 hours.'); return; }
    const params = { orgID: this.selected(), rosterID: detail.roster.id, body: { job_role_id: values.role, starts_at, ends_at } };
    const id = this.editing()?.id;
    this.mutate(id ? this.api.updateShift({ ...params, id }) : this.api.createShift(params), () => { this.editorOpen.set(false); this.editing.set(null); });
  }
  assign(event: Event, shift: Shift): void {
    const select = event.target as HTMLSelectElement, employee_id = select.value;
    select.value = shift.employee_id ?? '';
    const detail = this.detail(); if (!detail || this.busy() || employee_id === (shift.employee_id ?? '')) return;
    const params = { orgID: this.selected(), rosterID: detail.roster.id, id: shift.id };
    this.mutate(employee_id ? this.api.assignShift({ ...params, body: { employee_id } }) : this.api.unassignShift(params));
  }
  deleteShift(): void {
    const detail = this.detail(), shift = this.deleting(); if (!detail || !shift || this.busy()) return;
    this.busy.set(true); this.clearFeedback();
    this.api.deleteShift({ orgID: this.selected(), rosterID: detail.roster.id, id: shift.id }).pipe(timeout(15000), takeUntilDestroyed(this.destroyRef), finalize(() => this.busy.set(false))).subscribe({
      next: () => { this.detail.update(current => current ? { ...current, shifts: current.shifts.filter(item => item.id !== shift.id) } : null); this.deleting.set(null); this.notice.set('Shift removed.'); },
      error: error => this.handleError(error),
    });
  }
  private mutate(request: Observable<ShiftResult>, after?: () => void): void {
    this.busy.set(true); this.clearFeedback();
    request.pipe(timeout(15000), takeUntilDestroyed(this.destroyRef), finalize(() => this.busy.set(false))).subscribe({
      next: result => { this.detail.update(current => current ? { ...current, shifts: [...current.shifts.filter(shift => shift.id !== result.shift.id), result.shift].sort((a, b) => Date.parse(a.starts_at) - Date.parse(b.starts_at) || a.id.localeCompare(b.id)) } : null); this.violations.set(result.violations); this.notice.set('Schedule saved.'); after?.(); },
      error: error => this.handleError(error),
    });
  }
  private handleError(error: unknown): void {
    this.error.set(apiError(error));
    if (error instanceof HttpErrorResponse && Array.isArray(error.error?.violations)) this.violations.set(error.error.violations);
    this.checkSession(error);
  }
  private checkSession(error: unknown): void { if (error instanceof HttpErrorResponse && error.status === 401) { this.auth.identity.set(null); void this.router.navigateByUrl('/login'); } }
}
