import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { AuthService } from './auth.service';
import { provideApiConfiguration } from './api/api-configuration';
import { MyShifts } from './my-shifts';
import { Activity } from './activity';

const base = '/api/organizations/org-a';
describe('Published schedules and activity', () => {
  let http: HttpTestingController;
  beforeEach(() => {
    TestBed.configureTestingModule({ providers: [provideRouter([]), provideApiConfiguration(''), provideHttpClient(), provideHttpClientTesting()] });
    http = TestBed.inject(HttpTestingController);
    TestBed.inject(AuthService).identity.set({ user: { id: 'user-a', email: 'employee@example.com', display_name: 'Sam Taylor' }, memberships: [{ org_id: 'org-a', name: 'Team A', timezone: 'Africa/Lagos', role: 'employee' }], csrf_token: 'token' });
  });
  afterEach(() => http.verify());
  it('requests a bounded personal schedule without accepting an employee identifier', () => {
    const fixture = TestBed.createComponent(MyShifts);
    const request = http.expectOne(request => request.url === base + '/my-shifts');
    expect(request.request.params.has('employee_id')).toBe(false);
    expect(request.request.params.get('limit')).toBe('25');
    expect(request.request.params.get('from')).toContain('T');
    request.flush({ items: [{ id: 'shift-a', roster_id: 'roster-a', location_name: 'Main site', job_role_name: 'Nurse', starts_at: '2026-11-02T08:00:00Z', ends_at: '2026-11-02T16:00:00Z', timezone: 'Africa/Lagos', state: 'published' }], limit: 25, offset: 0, has_more: false });
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('Nurse');
    expect(fixture.nativeElement.textContent).toContain('09:00');
  });
  it('displays archived history and validates date ranges before another request', () => {
    const fixture = TestBed.createComponent(MyShifts);
    http.expectOne(request => request.url === base + '/my-shifts').flush({ items: [], limit: 25, offset: 0, has_more: false });
    fixture.componentInstance.filter(new Event('submit'), '2020-01-06', '2020-01-13');
    http.expectOne(request => request.url === base + '/my-shifts').flush({ items: [{ id: 'old-shift', roster_id: 'old-roster', location_name: 'Main site', job_role_name: 'Cashier', starts_at: '2020-01-06T08:00:00Z', ends_at: '2020-01-06T16:00:00Z', timezone: 'Africa/Lagos', state: 'archived' }], limit: 25, offset: 0, has_more: false });
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('ARCHIVED HISTORY');
    fixture.componentInstance.filter(new Event('submit'), '2026-11-03', '2026-11-02');
    http.expectNone(request => request.url === base + '/my-shifts');
    expect(fixture.componentInstance.error()).toContain('Choose a date range');
  });
  it('shows a manager activity entry using product labels and actor names', () => {
    TestBed.inject(AuthService).identity.update(identity => identity ? { ...identity, memberships: [{ ...identity.memberships[0], role: 'manager' }] } : null);
    const fixture = TestBed.createComponent(Activity);
    http.expectOne(request => request.url === base + '/activity').flush({ items: [{ id: 'event-a', action: 'roster.published', entity_type: 'roster', entity_id: 'roster-a', entity_label: 'Main site', actor_user_id: 'user-b', actor_name: 'Alex Morgan', created_at: '2026-11-02T08:00:00Z', before: {}, after: {} }], limit: 25, offset: 0, has_more: false });
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('Roster published');
    expect(fixture.nativeElement.textContent).toContain('Alex Morgan');
    expect(fixture.nativeElement.textContent).toContain('Main site');
  });
});
