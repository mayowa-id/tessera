import { TestBed } from '@angular/core/testing';
import { provideHttpClient, withInterceptors } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { AuthService, sessionInterceptor } from './auth.service';
import { provideApiConfiguration } from './api/api-configuration';
import { Roster, Shift } from './api/models';
import { Rosters } from './rosters';

const base = '/api/organizations/org-a';
const roster: Roster = { id: 'roster-a', location_id: 'site-a', location_name: 'Main site', week_start: '2026-11-02', timezone: 'Africa/Lagos', state: 'draft' };
const shift: Shift = { id: 'shift-a', job_role_id: 'role-a', job_role_name: 'Nurse', starts_at: '2026-11-02T08:00:00Z', ends_at: '2026-11-02T16:00:00Z', employee_id: null, employee_name: null };
const page = (items: unknown[]) => ({ items, offset: 0, limit: 100, has_more: false });

describe('Draft roster workflow', () => {
  let http: HttpTestingController;
  beforeEach(() => {
    TestBed.configureTestingModule({ providers: [provideRouter([]), provideApiConfiguration(''), provideHttpClient(withInterceptors([sessionInterceptor])), provideHttpClientTesting()] });
    http = TestBed.inject(HttpTestingController);
    TestBed.inject(AuthService).identity.set({ user: { id: 'manager-a', email: 'manager@example.com', display_name: 'Alex' }, memberships: [{ org_id: 'org-a', name: 'Team A', timezone: 'Africa/Lagos', role: 'manager' }], csrf_token: 'csrf-value' });
  });
  afterEach(() => http.verify());
  function start(shifts: Shift[] = [shift]) {
    const fixture = TestBed.createComponent(Rosters);
    http.expectOne(request => request.url === base + '/locations').flush(page([{ id: 'site-a', name: 'Main site', active: true }]));
    http.expectOne(request => request.url === base + '/job-roles').flush(page([{ id: 'role-a', name: 'Nurse', active: true }]));
    http.expectOne(request => request.url === base + '/employees').flush(page([{ id: 'person-a', display_name: 'Sam', active: true, job_role_ids: ['role-a'] }]));
    http.expectOne(request => request.url === base + '/rosters').flush(page([roster]));
    fixture.componentInstance.week.set('2026-11-02');
    fixture.componentInstance.openRoster(roster);
    http.expectOne(base + '/rosters/roster-a').flush({ roster, shifts });
    fixture.detectChanges();
    return fixture;
  }
  function selection(value: string): Event {
    const select = document.createElement('select');
    select.add(new Option('Unassigned', '')); select.add(new Option('Sam', 'person-a')); select.value = value;
    return { target: select } as unknown as Event;
  }
  it('creates a draft for the selected location and Monday', () => {
    const fixture = start();
    fixture.componentInstance.createRoster();
    const create = http.expectOne(base + '/rosters');
    expect(create.request.body).toEqual({ location_id: 'site-a', week_start: '2026-11-02' });
    expect(create.request.headers.get('X-CSRF-Token')).toBe('csrf-value');
    create.flush(roster);
    http.expectOne(request => request.url === base + '/rosters').flush(page([roster]));
    http.expectOne(base + '/rosters/roster-a').flush({ roster, shifts: [] });
    expect(fixture.componentInstance.detail()?.roster.id).toBe('roster-a');
  });
  it('converts organization-local times before creating a shift', () => {
    const fixture = start([]);
    fixture.componentInstance.openEditor();
    fixture.componentInstance.saveShift();
    const create = http.expectOne(base + '/rosters/roster-a/shifts');
    expect(create.request.body).toEqual({ job_role_id: 'role-a', starts_at: '2026-11-02T08:00:00.000Z', ends_at: '2026-11-02T16:00:00.000Z' });
    create.flush({ shift, violations: [] });
    expect(fixture.componentInstance.detail()?.shifts).toEqual([shift]);
    expect(fixture.componentInstance.editorOpen()).toBe(false);
  });
  it('retains the server-confirmed assignment when a drop-down change conflicts', () => {
    const fixture = start();
    const event = selection('person-a');
    fixture.componentInstance.assign(event, shift);
    expect((event.target as HTMLSelectElement).value).toBe('');
    expect(fixture.componentInstance.detail()?.shifts[0].employee_id).toBeNull();
    http.expectOne(base + '/rosters/roster-a/shifts/shift-a/assignment').flush({ message: 'The shift conflicts with scheduling rules.', violations: [{ code: 'MINIMUM_REST', severity: 'hard', message: 'Insufficient rest.' }] }, { status: 409, statusText: 'Conflict' });
    fixture.detectChanges();
    expect(fixture.componentInstance.detail()?.shifts[0].employee_id).toBeNull();
    expect(fixture.nativeElement.textContent).toContain('Insufficient rest.');
    expect(fixture.componentInstance.busy()).toBe(false);
  });
  it('accepts an assignment with warnings without treating it as a failure', () => {
    const fixture = start();
    fixture.componentInstance.assign(selection('person-a'), shift);
    http.expectOne(base + '/rosters/roster-a/shifts/shift-a/assignment').flush({ shift: { ...shift, employee_id: 'person-a', employee_name: 'Sam' }, violations: [{ code: 'WEEKLY_HOURS', severity: 'warning', message: 'Weekly hours exceeded.' }] });
    expect(fixture.componentInstance.detail()?.shifts[0].employee_id).toBe('person-a');
    expect(fixture.componentInstance.violations()[0].severity).toBe('warning');
    expect(fixture.componentInstance.error()).toBe('');
  });
  it('renders the confirmed employee after assignment, publication, and reopening', () => {
    const fixture = start();
    const select = fixture.nativeElement.querySelector('#assign-shift-a') as HTMLSelectElement;
    select.value = 'person-a'; select.dispatchEvent(new Event('change'));
    expect(select.value).toBe('');
    const assigned = { ...shift, employee_id: 'person-a', employee_name: 'Sam' };
    http.expectOne(base + '/rosters/roster-a/shifts/shift-a/assignment').flush({ shift: assigned, violations: [] });
    fixture.detectChanges();
    expect(select.value).toBe('person-a');
    fixture.componentInstance.detail.set({ roster: { ...roster, state: 'published' }, shifts: [assigned] });
    fixture.detectChanges();
    expect((fixture.nativeElement.querySelector('#assign-shift-a') as HTMLSelectElement).value).toBe('person-a');
    fixture.componentInstance.openRoster({ ...roster, state: 'published' });
    fixture.detectChanges();
    http.expectOne(base + '/rosters/roster-a').flush({ roster: { ...roster, state: 'published' }, shifts: [assigned] });
    fixture.detectChanges();
    expect((fixture.nativeElement.querySelector('#assign-shift-a') as HTMLSelectElement).value).toBe('person-a');
  });
  it('unassigns and removes a shift only after server confirmation', () => {
    const assigned = { ...shift, employee_id: 'person-a', employee_name: 'Sam' };
    const fixture = start([assigned]);
    fixture.componentInstance.assign(selection(''), assigned);
    const unassign = http.expectOne(base + '/rosters/roster-a/shifts/shift-a/assignment');
    expect(unassign.request.method).toBe('DELETE');
    unassign.flush({ shift, violations: [] });
    fixture.componentInstance.deleting.set(shift); fixture.componentInstance.deleteShift();
    expect(fixture.componentInstance.detail()?.shifts.length).toBe(1);
    const remove = http.expectOne(base + '/rosters/roster-a/shifts/shift-a');
    expect(remove.request.method).toBe('DELETE'); remove.flush(null);
    expect(fixture.componentInstance.detail()?.shifts.length).toBe(0);
  });
  it('rejects a non-Monday week before requesting a roster list', () => {
    const fixture = start();
    fixture.componentInstance.changeWeek('2026-11-03');
    http.expectNone(request => request.url === base + '/rosters');
    expect(fixture.componentInstance.error()).toContain('Choose a Monday');
  });
  it('publishes a fully assigned draft and adopts the complete validated snapshot', () => {
    const assigned = { ...shift, employee_id: 'person-a', employee_name: 'Sam' };
    const fixture = start([assigned]);
    expect(fixture.componentInstance.readyToPublish()).toBe(true);
    fixture.componentInstance.lifecycleAction.set('publish');
    fixture.componentInstance.transition();
    const publish = http.expectOne(base + '/rosters/roster-a/publish');
    expect(publish.request.method).toBe('POST');
    expect(publish.request.headers.get('X-CSRF-Token')).toBe('csrf-value');
    publish.flush({ detail: { roster: { ...roster, state: 'published' }, shifts: [assigned] }, violations: [] });
    fixture.detectChanges();
    expect(fixture.componentInstance.detail()?.roster.state).toBe('published');
    expect(fixture.componentInstance.readyToPublish()).toBe(false);
    expect(fixture.componentInstance.notice()).toContain('Employees can now see');
  });
  it('keeps the draft editable when publication validation fails', () => {
    const fixture = start([{ ...shift, employee_id: 'person-a', employee_name: 'Sam' }]);
    fixture.componentInstance.lifecycleAction.set('publish');
    fixture.componentInstance.transition();
    http.expectOne(base + '/rosters/roster-a/publish').flush({ message: 'The shift conflicts with scheduling rules.', violations: [{ code: 'QUALIFICATION_REQUIRED', severity: 'hard', message: 'Qualification is no longer held.', employee_id: 'person-a', shift_id: shift.id }] }, { status: 409, statusText: 'Conflict' });
    fixture.detectChanges();
    expect(fixture.componentInstance.detail()?.roster.state).toBe('draft');
    expect(fixture.componentInstance.busy()).toBe(false);
    expect(fixture.nativeElement.textContent).toContain('Sam: Qualification is no longer held.');
  });
  it('archives an ended published roster while keeping its history', () => {
    const fixture = start();
    fixture.componentInstance.detail.set({ roster: { ...roster, week_start: '2020-01-06', state: 'published' }, shifts: [] });
    expect(fixture.componentInstance.archiveReady()).toBe(true);
    fixture.componentInstance.lifecycleAction.set('archive');
    fixture.componentInstance.transition();
    http.expectOne(base + '/rosters/roster-a/archive').flush({ detail: { roster: { ...roster, week_start: '2020-01-06', state: 'archived' }, shifts: [] }, violations: [] });
    expect(fixture.componentInstance.detail()?.roster.state).toBe('archived');
  });
});
