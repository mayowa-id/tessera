import { TestBed } from '@angular/core/testing';
import { provideHttpClient, withInterceptors } from '@angular/common/http';
import { HttpTestingController } from '@angular/common/http/testing';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { AuthService, sessionInterceptor } from './auth.service';
import { provideApiConfiguration } from './api/api-configuration';
import { Staff } from './staff';

const base = '/api/organizations/org-a';
const role = { id: 'role-a', name: 'Nurse', active: true };
const employee = { id: 'person-a', display_name: 'Sam Taylor', active: true, job_role_ids: ['role-a'] };
const page = (items: unknown[], offset = 0, limit = 25, has_more = false) => ({ items, offset, limit, has_more });

describe('Staff administration', () => {
  let http: HttpTestingController;
  beforeEach(() => {
    TestBed.configureTestingModule({ providers: [provideRouter([]), provideApiConfiguration(''), provideHttpClient(withInterceptors([sessionInterceptor])), provideHttpClientTesting()] });
    http = TestBed.inject(HttpTestingController);
    TestBed.inject(AuthService).identity.set({ user: { id: 'manager-a', email: 'manager@example.com', display_name: 'Alex' }, memberships: [{ org_id: 'org-a', name: 'Team A', timezone: 'Africa/Lagos', role: 'manager' }], csrf_token: 'csrf-value' });
  });
  afterEach(() => http.verify());
  function start() {
    const fixture = TestBed.createComponent(Staff);
    http.expectOne(request => request.url === base + '/job-roles').flush(page([role], 0, 100));
    http.expectOne(request => request.url === base + '/employees').flush(page([employee]));
    fixture.detectChanges();
    return fixture;
  }

  it('creates a qualified employee with CSRF headers and reloads the list', () => {
    const fixture = start();
    fixture.componentInstance.openEditor();
    fixture.componentInstance.form.setValue({ name: ' Jamie Lane ', active: true });
    fixture.componentInstance.selectedRoles.set(['role-a']);
    fixture.componentInstance.save();
    const create = http.expectOne(base + '/employees');
    expect(create.request.method).toBe('POST');
    expect(create.request.headers.get('X-CSRF-Token')).toBe('csrf-value');
    expect(create.request.body).toEqual({ display_name: 'Jamie Lane', active: true, job_role_ids: ['role-a'] });
    create.flush({ ...employee, id: 'new-person', display_name: 'Jamie Lane' });
    http.expectOne(request => request.url === base + '/employees').flush(page([employee]));
    expect(fixture.componentInstance.editorOpen()).toBe(false);
    expect(fixture.componentInstance.notice()).toBe('Record created.');
  });

  it('deactivates an employee while preserving qualifications', () => {
    const fixture = start();
    fixture.componentInstance.openEditor(employee);
    fixture.componentInstance.form.controls.active.setValue(false);
    fixture.componentInstance.save();
    const update = http.expectOne(base + '/employees/person-a');
    expect(update.request.method).toBe('PUT');
    expect(update.request.body.job_role_ids).toEqual(['role-a']);
    expect(update.request.body.active).toBe(false);
    update.flush({ ...employee, active: false });
    http.expectOne(request => request.url === base + '/employees').flush(page([]));
    expect(fixture.componentInstance.notice()).toBe('Changes saved.');
  });

  it('keeps rejected edits open and displays the server validation', () => {
    const fixture = start();
    fixture.componentInstance.openEditor(employee);
    fixture.componentInstance.save();
    http.expectOne(base + '/employees/person-a').flush({ message: 'Choose roles from this organization.' }, { status: 422, statusText: 'Unprocessable Content' });
    fixture.detectChanges();
    expect(fixture.componentInstance.editorOpen()).toBe(true);
    expect(fixture.componentInstance.busy()).toBe(false);
    expect(fixture.nativeElement.textContent).toContain('Choose roles from this organization.');
  });

  it('loads all role pages and permits retaining only existing inactive qualifications', () => {
    const fixture = TestBed.createComponent(Staff);
    http.expectOne(request => request.url === base + '/job-roles').flush(page([role], 0, 100, true));
    const next = http.expectOne(request => request.url === base + '/job-roles');
    expect(next.request.params.get('offset')).toBe('100');
    const inactive = { id: 'role-b', name: 'Retired role', active: false };
    next.flush(page([inactive], 100, 100));
    http.expectOne(request => request.url === base + '/employees').flush(page([employee]));
    expect(fixture.componentInstance.roles().length).toBe(2);
    fixture.componentInstance.openEditor(employee);
    expect(fixture.componentInstance.roleDisabled(inactive)).toBe(true);
    fixture.componentInstance.openEditor({ ...employee, job_role_ids: ['role-b'] });
    expect(fixture.componentInstance.roleDisabled(inactive)).toBe(false);
  });

  it('paginates and searches while requesting inactive records explicitly', () => {
    const fixture = start();
    fixture.componentInstance.paginate(1);
    const next = http.expectOne(request => request.url === base + '/employees');
    expect(next.request.params.get('offset')).toBe('25');
    next.flush(page([employee], 25));
    fixture.componentInstance.includeInactive.set(true);
    fixture.componentInstance.applySearch(new Event('submit'), ' Sam ');
    const search = http.expectOne(request => request.url === base + '/employees');
    expect(search.request.params.get('offset')).toBe('0');
    expect(search.request.params.get('q')).toBe('Sam');
    expect(search.request.params.get('include_inactive')).toBe('true');
    search.flush(page([employee]));
  });

  it('creates a location and shows duplicate-name conflicts', () => {
    const fixture = start();
    fixture.componentInstance.selectTab('locations');
    http.expectOne(request => request.url === base + '/locations').flush(page([]));
    fixture.componentInstance.openEditor();
    fixture.componentInstance.form.setValue({ name: 'Main site', active: true });
    fixture.componentInstance.save();
    http.expectOne(base + '/locations').flush({ message: 'That name is already in use in this organization.' }, { status: 409, statusText: 'Conflict' });
    expect(fixture.componentInstance.formError()).toContain('already in use');
  });
});
