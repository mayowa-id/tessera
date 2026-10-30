import { Component, DestroyRef, ElementRef, computed, inject, signal, viewChild } from '@angular/core';
import { HttpErrorResponse } from '@angular/common/http';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { EMPTY, expand, finalize, reduce, timeout } from 'rxjs';
import { AuthService, apiError } from './auth.service';
import { TimeOffService } from './api/services/time-off.service';
import { StaffService } from './api/services/staff.service';
import { Employee, TimeOff as TimeOffRecord, TimeOffInput, TimeOffShiftConflict } from './api/models';
import { localInstant, timeCandidates, wallTime } from './roster-time';

type Action = 'approve' | 'reject' | 'cancel';
function addDays(date: string, count: number): string {
  const value = new Date(date + 'T12:00:00Z');
  value.setUTCDate(value.getUTCDate() + count);
  return value.toISOString().slice(0, 10);
}

@Component({
  selector: 'app-time-off',
  imports: [ReactiveFormsModule, RouterLink],
  templateUrl: './time-off.html',
  styleUrl: './time-off.css',
})
export class TimeOff {
  readonly auth = inject(AuthService);
  private readonly api = inject(TimeOffService);
  private readonly staff = inject(StaffService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);
  private readonly destroyRef = inject(DestroyRef);
  private readonly dialog = viewChild.required<ElementRef<HTMLDialogElement>>('decisionDialog');
  readonly memberships = computed(() => this.auth.identity()?.memberships ?? []);
  readonly selected = signal(
    this.memberships().find((m) => m.org_id === this.route.snapshot.queryParamMap.get('org'))?.org_id ??
      this.memberships()[0]?.org_id ??
      '',
  );
  readonly membership = computed(() => this.memberships().find((m) => m.org_id === this.selected()));
  readonly isManager = computed(() => ['admin', 'manager'].includes(this.membership()?.role ?? ''));
  readonly zone = computed(() => this.membership()?.timezone ?? 'Africa/Lagos');
  readonly mode = signal<'mine' | 'team'>('mine');
  readonly items = signal<TimeOffRecord[]>([]);
  readonly employees = signal<Employee[]>([]);
  readonly loading = signal(false);
  readonly busy = signal(false);
  readonly staffLoading = signal(false);
  readonly staffError = signal('');
  readonly error = signal('');
  readonly notice = signal('');
  readonly offset = signal(0);
  readonly hasMore = signal(false);
  readonly conflicts = signal<TimeOffShiftConflict[]>([]);
  readonly fromDate = signal(wallTime(new Date(), this.zone()).slice(0, 10));
  readonly throughDate = signal(addDays(this.fromDate(), 30));
  readonly status = signal<TimeOffRecord['status'] | ''>('');
  readonly decision = signal<{ item: TimeOffRecord; action: Action } | null>(null);
  readonly decisionNote = signal('');
  readonly form = inject(FormBuilder).nonNullable.group({
    kind: ['leave', Validators.required],
    starts_at: ['', Validators.required],
    ends_at: ['', Validators.required],
    reason: ['', Validators.maxLength(500)],
    employee_id: [''],
    start_occurrence: ['earlier'],
    end_occurrence: ['later'],
  });
  private version = 0;
  private staffVersion = 0;

  constructor() {
    this.resetTimes();
    this.load();
  }
  private headers() {
    return { 'X-Tessera-Request': '1' as const, 'X-CSRF-Token': this.auth.identity()?.csrf_token ?? '' };
  }
  private resetTimes(): void {
    const next = addDays(wallTime(new Date(), this.zone()).slice(0, 10), 1);
    this.form.patchValue({
      starts_at: next + 'T09:00',
      ends_at: next + 'T17:00',
      reason: '',
      employee_id: '',
      start_occurrence: 'earlier',
      end_occurrence: 'later',
    });
  }
  changeOrganization(event: Event): void {
    if (this.busy()) return;
    this.selected.set((event.target as HTMLSelectElement).value);
    this.mode.set('mine');
    this.status.set('');
    this.offset.set(0);
    this.employees.set([]);
    this.staffVersion++;
    this.staffLoading.set(false);
    this.staffError.set('');
    this.notice.set('');
    this.fromDate.set(wallTime(new Date(), this.zone()).slice(0, 10));
    this.throughDate.set(addDays(this.fromDate(), 30));
    this.resetTimes();
    this.load();
    void this.router.navigate([], {
      relativeTo: this.route,
      queryParams: { org: this.selected() },
      replaceUrl: true,
    });
  }
  setMode(mode: 'mine' | 'team'): void {
    if (this.busy() || (mode === 'team' && !this.isManager())) return;
    this.mode.set(mode);
    this.status.set(mode === 'team' ? 'pending' : '');
    this.offset.set(0);
    this.form.controls.employee_id.reset();
    this.notice.set('');
    if (mode === 'team') this.loadStaff();
    this.load();
  }
  loadStaff(): void {
    if (!this.isManager()) return;
    const version = ++this.staffVersion,
      orgID = this.selected();
    this.staffLoading.set(true);
    this.staffError.set('');
    this.employees.set([]);
    const page = (offset: number) => this.staff.listEmployees({ orgID, limit: 100, offset });
    page(0)
      .pipe(
        expand((value) => (value.has_more ? page(value.offset + value.limit) : EMPTY)),
        reduce((all, value) => all.concat(value.items), [] as Employee[]),
        timeout(15000),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe({
        next: (items) => {
          if (version === this.staffVersion) {
            this.employees.set(items.filter((e) => e.active));
            this.staffLoading.set(false);
          }
        },
        error: (error) => {
          if (version === this.staffVersion) {
            this.staffError.set(apiError(error));
            this.staffLoading.set(false);
          }
        },
      });
  }
  filter(event: Event, from: string, through: string, status: string): void {
    event.preventDefault();
    if (this.busy()) return;
    this.fromDate.set(from);
    this.throughDate.set(through);
    this.status.set(status as TimeOffRecord['status'] | '');
    this.offset.set(0);
    this.load();
  }
  paginate(direction: number): void {
    if (this.busy()) return;
    this.offset.update((offset) => Math.max(0, offset + direction * 25));
    this.load();
  }
  load(): void {
    const version = ++this.version;
    this.items.set([]);
    this.error.set('');
    this.conflicts.set([]);
    this.hasMore.set(false);
    if (!this.membership()) {
      this.loading.set(false);
      return;
    }
    let from: string, to: string;
    try {
      from = localInstant(this.fromDate() + 'T00:00', this.zone(), 'earlier');
      to = localInstant(addDays(this.throughDate(), 1) + 'T00:00', this.zone(), 'later');
      if (Date.parse(to) <= Date.parse(from) || Date.parse(to) - Date.parse(from) > 93 * 86400000)
        throw new Error('Choose a date range of up to three months.');
    } catch (error) {
      this.loading.set(false);
      this.error.set(error instanceof Error ? error.message : 'Choose valid dates.');
      return;
    }
    this.loading.set(true);
    const params = {
      orgID: this.selected(),
      from,
      to,
      limit: 25,
      offset: this.offset(),
      status: this.status() || undefined,
    };
    const request =
      this.mode() === 'team' && this.isManager()
        ? this.api.listTeamTimeOff(params)
        : this.api.listMyTimeOff(params);
    request.pipe(timeout(15000), takeUntilDestroyed(this.destroyRef)).subscribe({
      next: (page) => {
        if (version === this.version) {
          this.items.set(page.items);
          this.hasMore.set(page.has_more);
          this.loading.set(false);
        }
      },
      error: (error) => {
        if (version === this.version) {
          this.loading.set(false);
          this.handleError(error);
        }
      },
    });
  }
  ambiguous(value: string): boolean {
    return timeCandidates(value, this.zone()).length > 1;
  }
  submit(): void {
    if (this.busy()) return;
    this.error.set('');
    this.notice.set('');
    this.conflicts.set([]);
    if (this.form.invalid) {
      this.form.markAllAsTouched();
      this.error.set('Enter start and end times. Notes may contain up to 500 characters.');
      return;
    }
    const value = this.form.getRawValue();
    let body: TimeOffInput;
    try {
      const start = localInstant(value.starts_at, this.zone(), value.start_occurrence),
        end = localInstant(value.ends_at, this.zone(), value.end_occurrence);
      if (
        Date.parse(end) <= Date.parse(start) ||
        Date.parse(end) - Date.parse(start) > 366 * 86400000 ||
        Date.parse(end) <= Date.now()
      )
        throw new Error('End time must be after start and in the future, within 366 elapsed days.');
      if (value.kind !== 'leave' && value.kind !== 'unavailable')
        throw new Error('Choose leave or unavailable time.');
      body = { kind: value.kind, starts_at: start, ends_at: end, reason: value.reason };
    } catch (error) {
      this.error.set(error instanceof Error ? error.message : 'Check the times.');
      return;
    }
    const orgID = this.selected();
    this.busy.set(true);
    const request =
      this.mode() === 'team' && this.isManager() && value.employee_id
        ? this.api.createEmployeeTimeOff({ orgID, employeeID: value.employee_id, body, ...this.headers() })
        : this.api.createMyTimeOff({ orgID, body, ...this.headers() });
    request
      .pipe(
        timeout(15000),
        takeUntilDestroyed(this.destroyRef),
        finalize(() => this.busy.set(false)),
      )
      .subscribe({
        next: (result) => {
          this.notice.set(
            result.kind === 'leave'
              ? 'Leave requested. It will block assignments once approved.'
              : 'Unavailable time saved. Overlapping assignments are now blocked.',
          );
          this.form.controls.reason.reset();
          this.fromDate.set(wallTime(result.starts_at, this.zone()).slice(0, 10));
          this.throughDate.set(addDays(this.fromDate(), 30));
          this.offset.set(0);
          if (result.kind === 'unavailable') this.status.set('');
          this.load();
        },
        error: (error) => this.handleError(error),
      });
  }
  choose(item: TimeOffRecord, action: Action): void {
    if (this.busy()) return;
    this.decision.set({ item, action });
    this.decisionNote.set('');
    this.error.set('');
    this.conflicts.set([]);
    this.dialog().nativeElement.showModal();
  }
  closeDecision(): void {
    if (!this.busy()) {
      this.dialog().nativeElement.close();
      this.decision.set(null);
    }
  }
  confirm(): void {
    const choice = this.decision();
    if (!choice || this.busy()) return;
    if (this.decisionNote().length > 500) {
      this.error.set('Keep the decision note within 500 characters.');
      return;
    }
    this.busy.set(true);
    this.error.set('');
    this.notice.set('');
    this.conflicts.set([]);
    const params = {
      orgID: this.selected(),
      id: choice.item.id,
      body: { note: this.decisionNote() },
      ...this.headers(),
    };
    const request =
      choice.action === 'approve'
        ? this.api.approveTimeOff(params)
        : choice.action === 'reject'
          ? this.api.rejectTimeOff(params)
          : this.api.cancelTimeOff(params);
    request
      .pipe(
        timeout(15000),
        takeUntilDestroyed(this.destroyRef),
        finalize(() => this.busy.set(false)),
      )
      .subscribe({
        next: (result) => {
          this.dialog().nativeElement.close();
          this.decision.set(null);
          this.notice.set('Request ' + result.status + '.');
          this.load();
        },
        error: (error) => {
          this.dialog().nativeElement.close();
          this.decision.set(null);
          this.handleError(error);
        },
      });
  }
  display(value: string): string {
    return new Intl.DateTimeFormat('en-GB', {
      timeZone: this.zone(),
      dateStyle: 'medium',
      timeStyle: 'short',
    }).format(new Date(value));
  }
  private handleError(error: unknown): void {
    this.error.set(apiError(error));
    if (error instanceof HttpErrorResponse) {
      if (Array.isArray(error.error?.conflicting_shifts)) this.conflicts.set(error.error.conflicting_shifts);
      if (error.status === 401) {
        this.auth.identity.set(null);
        void this.router.navigateByUrl('/login');
      }
    }
  }
}
