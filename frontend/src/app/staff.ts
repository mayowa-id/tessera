import { Component, DestroyRef, computed, inject, signal } from '@angular/core';
import { HttpErrorResponse } from '@angular/common/http';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { EMPTY, Observable, expand, finalize, reduce, timeout } from 'rxjs';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { AuthService, apiError } from './auth.service';
import { StaffService } from './api/services/staff.service';
import { CatalogInput, CatalogItem, CatalogPage, Employee, EmployeeInput, EmployeePage } from './api/models';

type Tab = 'employees' | 'locations' | 'job-roles';
type RecordItem = Employee | CatalogItem;

@Component({ selector: 'app-staff', imports: [ReactiveFormsModule, RouterLink], templateUrl: './staff.html' })
export class Staff {
  readonly auth = inject(AuthService);
  private readonly api = inject(StaffService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);
  private readonly destroyRef = inject(DestroyRef);
  readonly memberships = computed(() => (this.auth.identity()?.memberships ?? []).filter(org => org.role === 'admin' || org.role === 'manager'));
  readonly selected = signal(this.memberships().find(org => org.org_id === this.route.snapshot.queryParamMap.get('org'))?.org_id ?? this.memberships()[0]?.org_id ?? '');
  readonly membership = computed(() => this.memberships().find(org => org.org_id === this.selected()));
  readonly tab = signal<Tab>('employees');
  readonly tabs: { value: Tab; label: string }[] = [{ value: 'employees', label: 'Team members' }, { value: 'locations', label: 'Locations' }, { value: 'job-roles', label: 'Job roles' }];
  readonly title = computed(() => this.tabs.find(tab => tab.value === this.tab())!.label);
  readonly singular = computed(() => this.tab() === 'employees' ? 'team member' : this.tab() === 'locations' ? 'location' : 'job role');
  readonly items = signal<RecordItem[]>([]);
  readonly roles = signal<CatalogItem[]>([]);
  readonly rolesReady = signal(false);
  readonly rolesError = signal('');
  readonly loading = signal(false);
  readonly busy = signal(false);
  readonly error = signal('');
  readonly notice = signal('');
  readonly formError = signal('');
  readonly offset = signal(0);
  readonly hasMore = signal(false);
  readonly search = signal('');
  readonly includeInactive = signal(false);
  readonly editorOpen = signal(false);
  readonly editing = signal<RecordItem | null>(null);
  readonly selectedRoles = signal<string[]>([]);
  readonly form = inject(FormBuilder).nonNullable.group({ name: ['', [Validators.required, Validators.maxLength(120)]], active: [true] });
  private requestVersion = 0;
  private rolesVersion = 0;

  constructor() { this.loadRoles(); this.load(); }
  selectOrganization(event: Event): void {
    this.selected.set((event.target as HTMLSelectElement).value);
    this.offset.set(0); this.search.set(''); this.notice.set(''); this.cancel();
    void this.router.navigate([], { relativeTo: this.route, queryParams: { org: this.selected() }, replaceUrl: true });
    this.loadRoles(); this.load();
  }
  selectTab(tab: Tab): void {
    if (this.busy()) return;
    this.tab.set(tab); this.offset.set(0); this.search.set(''); this.notice.set(''); this.cancel(); this.load();
  }
  load(): void {
    const version = ++this.requestVersion;
    this.loading.set(true); this.error.set(''); this.items.set([]); this.hasMore.set(false);
    const params = { orgID: this.selected(), limit: 25, offset: this.offset(), q: this.search(), include_inactive: this.includeInactive() };
    const result: Observable<EmployeePage | CatalogPage> = this.tab() === 'employees' ? this.api.listEmployees(params) : this.tab() === 'locations' ? this.api.listLocations(params) : this.api.listJobRoles(params);
    result.pipe(timeout(15000), takeUntilDestroyed(this.destroyRef)).subscribe({
      next: page => { if (version !== this.requestVersion) return; this.items.set(page.items); this.hasMore.set(page.has_more); this.loading.set(false); },
      error: error => { if (version !== this.requestVersion) return; this.loading.set(false); this.error.set(apiError(error)); this.checkSession(error); },
    });
  }
  loadRoles(): void {
    const version = ++this.rolesVersion;
    const orgID = this.selected();
    this.roles.set([]); this.rolesReady.set(false); this.rolesError.set('');
    this.api.listJobRoles({ orgID, limit: 100, offset: 0, include_inactive: true }).pipe(
      expand(page => page.has_more ? this.api.listJobRoles({ orgID, limit: 100, offset: page.offset + page.limit, include_inactive: true }) : EMPTY),
      reduce((roles, page) => roles.concat(page.items), [] as CatalogItem[]), timeout(30000), takeUntilDestroyed(this.destroyRef),
    ).subscribe({
      next: roles => { if (version !== this.rolesVersion) return; this.roles.set(roles); this.rolesReady.set(true); },
      error: error => { if (version !== this.rolesVersion) return; this.rolesError.set(apiError(error)); this.checkSession(error); },
    });
  }
  applySearch(event: Event, value: string): void { event.preventDefault(); this.search.set(value.trim()); this.offset.set(0); this.load(); }
  toggleInactive(event: Event): void { this.includeInactive.set((event.target as HTMLInputElement).checked); this.offset.set(0); this.load(); }
  paginate(direction: number): void { this.offset.update(offset => Math.max(0, offset + direction * 25)); this.load(); }
  name(item: RecordItem): string { return 'display_name' in item ? item.display_name : item.name; }
  qualifications(item: RecordItem): string { return 'job_role_ids' in item ? item.job_role_ids.map(id => this.roles().find(role => role.id === id)?.name ?? 'Unavailable role').join(', ') || 'No qualifications yet' : ''; }
  openEditor(item: RecordItem | null = null): void {
    this.editing.set(item); this.form.reset({ name: item ? this.name(item) : '', active: item?.active ?? true });
    this.selectedRoles.set(item && 'job_role_ids' in item ? [...item.job_role_ids] : []);
    this.formError.set(''); this.notice.set(''); this.editorOpen.set(true);
  }
  cancel(): void { if (this.busy()) return; this.editorOpen.set(false); this.editing.set(null); this.formError.set(''); }
  roleDisabled(role: CatalogItem): boolean {
    const original = this.editing();
    return !role.active && !(original && 'job_role_ids' in original && original.job_role_ids.includes(role.id));
  }
  toggleRole(event: Event, id: string): void {
    const checked = (event.target as HTMLInputElement).checked;
    this.selectedRoles.update(roles => checked ? [...roles, id] : roles.filter(role => role !== id));
  }
  save(): void {
    if (this.busy()) return;
    const { name, active } = this.form.getRawValue();
    if (this.form.invalid || !name.trim()) { this.form.markAllAsTouched(); this.formError.set('Enter a name of 1 to 120 characters.'); return; }
    if (this.tab() === 'employees' && !this.rolesReady()) { this.formError.set('Wait for job roles to load before saving.'); return; }
    const orgID = this.selected(), id = this.editing()?.id;
    const body: EmployeeInput = { display_name: name.trim(), active, job_role_ids: this.selectedRoles() };
    const catalogBody: CatalogInput = { name: name.trim(), active };
    let result: Observable<Employee | CatalogItem>;
    if (this.tab() === 'employees') result = id ? this.api.updateEmployee({ orgID, id, body }) : this.api.createEmployee({ orgID, body });
    else if (this.tab() === 'locations') result = id ? this.api.updateLocation({ orgID, id, body: catalogBody }) : this.api.createLocation({ orgID, body: catalogBody });
    else result = id ? this.api.updateJobRole({ orgID, id, body: catalogBody }) : this.api.createJobRole({ orgID, body: catalogBody });
    this.busy.set(true); this.formError.set('');
    result.pipe(timeout(15000), takeUntilDestroyed(this.destroyRef), finalize(() => this.busy.set(false))).subscribe({
      next: () => {
        this.editorOpen.set(false); this.editing.set(null); this.notice.set(id ? 'Changes saved.' : 'Record created.'); this.offset.set(0);
        if (this.tab() === 'job-roles') this.loadRoles(); this.load();
      },
      error: error => { this.formError.set(apiError(error)); this.checkSession(error); },
    });
  }
  private checkSession(error: unknown): void {
    if (error instanceof HttpErrorResponse && error.status === 401) { this.auth.identity.set(null); void this.router.navigateByUrl('/login'); }
  }
}
