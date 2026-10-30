import { TestBed } from '@angular/core/testing';
import { provideHttpClient, withInterceptors } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { vi } from 'vitest';
import { AuthService, sessionInterceptor } from './auth.service';
import { provideApiConfiguration } from './api/api-configuration';
import { TimeOff as Record } from './api/models';
import { TimeOff } from './time-off';

const base = '/api/organizations/org-a';
const item: Record = {
  id: 'request-a',
  employee_id: 'employee-a',
  employee_name: 'Sam',
  kind: 'leave',
  status: 'pending',
  starts_at: '2030-11-04T08:00:00Z',
  ends_at: '2030-11-04T16:00:00Z',
  reason: 'Family time',
  decision_note: '',
  created_at: '2026-10-30T00:00:00Z',
  updated_at: '2026-10-30T00:00:00Z',
};
const page = (items: unknown[]) => ({ items, offset: 0, limit: 25, has_more: false });

describe('Availability and leave', () => {
  let http: HttpTestingController;
  let auth: AuthService;
  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        provideApiConfiguration(''),
        provideHttpClient(withInterceptors([sessionInterceptor])),
        provideHttpClientTesting(),
      ],
    });
    http = TestBed.inject(HttpTestingController);
    auth = TestBed.inject(AuthService);
    auth.identity.set({
      user: { id: 'user-a', email: 'sam@example.com', display_name: 'Sam' },
      memberships: [{ org_id: 'org-a', name: 'Team A', timezone: 'Africa/Lagos', role: 'employee' }],
      csrf_token: 'csrf',
    });
  });
  afterEach(() => http.verify());
  function start(items: Record[] = []) {
    const fixture = TestBed.createComponent(TimeOff);
    http.expectOne((r) => r.url === base + '/my-time-off').flush(page(items));
    fixture.detectChanges();
    return fixture;
  }
  function setForm(component: TimeOff) {
    component.form.patchValue({
      starts_at: '2030-11-04T09:00',
      ends_at: '2030-11-04T17:00',
      reason: 'Family time',
    });
  }
  function dialog(fixture: ReturnType<typeof start>) {
    const value = fixture.nativeElement.querySelector('dialog') as HTMLDialogElement;
    value.showModal = vi.fn();
    value.close = vi.fn();
    return value;
  }

  it('loads only your records and does not expose manager review actions to employees', () => {
    const fixture = start([item]);
    expect(fixture.nativeElement.textContent).toContain('Family time');
    expect(fixture.nativeElement.textContent).not.toContain('Team requests');
    expect(
      Array.from(fixture.nativeElement.querySelectorAll('button') as NodeListOf<HTMLButtonElement>).some(
        (b) => b.textContent === 'Approve',
      ),
    ).toBe(false);
    http.expectNone((r) => r.url === base + '/employees' || r.url === base + '/time-off');
  });
  it('sends a personal request in the organization timezone and waits for manager approval', () => {
    const fixture = start();
    setForm(fixture.componentInstance);
    fixture.componentInstance.submit();
    const request = http.expectOne(base + '/my-time-off');
    expect(request.request.body).toEqual({
      kind: 'leave',
      starts_at: '2030-11-04T08:00:00.000Z',
      ends_at: '2030-11-04T16:00:00.000Z',
      reason: 'Family time',
    });
    expect(request.request.headers.get('X-CSRF-Token')).toBe('csrf');
    request.flush(item);
    http.expectOne((r) => r.url === base + '/my-time-off').flush(page([item]));
    expect(fixture.componentInstance.notice()).toContain('once approved');
    expect(fixture.componentInstance.items()[0].status).toBe('pending');
  });
  it('requires confirmation before cancellation and refreshes the saved state', () => {
    const fixture = start([item]);
    const modal = dialog(fixture);
    fixture.componentInstance.choose(item, 'cancel');
    expect(modal.showModal).toHaveBeenCalledOnce();
    http.expectNone(base + '/time-off/request-a/cancel');
    fixture.componentInstance.confirm();
    http.expectOne(base + '/time-off/request-a/cancel').flush({ ...item, status: 'cancelled' });
    http.expectOne((r) => r.url === base + '/my-time-off').flush(page([{ ...item, status: 'cancelled' }]));
    expect(modal.close).toHaveBeenCalledOnce();
    expect(fixture.componentInstance.items()[0].status).toBe('cancelled');
  });
  it('lets a manager review requests and shows assigned-shift conflicts without pretending approval succeeded', () => {
    auth.identity.update((value) => ({
      ...value!,
      memberships: [{ ...value!.memberships[0], role: 'manager' }],
    }));
    const fixture = start();
    fixture.componentInstance.setMode('team');
    http
      .expectOne((r) => r.url === base + '/employees')
      .flush(page([{ id: 'employee-a', display_name: 'Sam', active: true, job_role_ids: [] }]));
    http
      .expectOne((r) => r.url === base + '/time-off' && r.params.get('status') === 'pending')
      .flush(page([item]));
    dialog(fixture);
    fixture.componentInstance.choose(item, 'approve');
    fixture.componentInstance.confirm();
    http
      .expectOne(base + '/time-off/request-a/approve')
      .flush(
        {
          message: 'Resolve assigned shifts first.',
          conflicting_shifts: [
            {
              id: 'shift-a',
              roster_id: 'roster-a',
              state: 'draft',
              week_start: '2030-11-04',
              location_name: 'Main site',
              starts_at: item.starts_at,
              ends_at: item.ends_at,
            },
          ],
        },
        { status: 409, statusText: 'Conflict' },
      );
    fixture.detectChanges();
    expect(fixture.componentInstance.items()[0].status).toBe('pending');
    expect(fixture.nativeElement.textContent).toContain('These shifts need attention first');
    expect(fixture.nativeElement.querySelector('a[href*="week=2030-11-04"]')).toBeTruthy();
  });
  it('allows managers to record time for staff without a login using the employee endpoint', () => {
    auth.identity.update((value) => ({
      ...value!,
      memberships: [{ ...value!.memberships[0], role: 'admin' }],
    }));
    const fixture = start();
    fixture.componentInstance.setMode('team');
    http
      .expectOne((r) => r.url === base + '/employees')
      .flush(page([{ id: 'unlinked', display_name: 'Robin', active: true, job_role_ids: [] }]));
    http.expectOne((r) => r.url === base + '/time-off').flush(page([]));
    setForm(fixture.componentInstance);
    fixture.componentInstance.form.patchValue({ employee_id: 'unlinked', kind: 'unavailable' });
    fixture.componentInstance.submit();
    http
      .expectOne(base + '/employees/unlinked/time-off')
      .flush({ ...item, employee_id: 'unlinked', kind: 'unavailable', status: 'approved' });
    http.expectOne((r) => r.url === base + '/time-off').flush(page([]));
    expect(fixture.componentInstance.notice()).toContain('now blocked');
  });
  it('rejects skipped daylight-saving minutes before making a request', () => {
    auth.identity.update((value) => ({
      ...value!,
      memberships: [{ ...value!.memberships[0], timezone: 'America/New_York' }],
    }));
    const fixture = start();
    fixture.componentInstance.form.patchValue({ starts_at: '2026-03-08T02:30', ends_at: '2026-03-08T04:00' });
    fixture.componentInstance.submit();
    expect(fixture.componentInstance.error()).toContain('does not exist');
    http.expectNone(base + '/my-time-off');
  });
  it('ignores stale responses when changing organizations', () => {
    auth.identity.update((value) => ({
      ...value!,
      memberships: [
        ...value!.memberships,
        { org_id: 'org-b', name: 'Team B', timezone: 'Europe/London', role: 'employee' },
      ],
    }));
    const fixture = TestBed.createComponent(TimeOff);
    const old = http.expectOne((r) => r.url === base + '/my-time-off');
    fixture.componentInstance.changeOrganization({ target: { value: 'org-b' } } as unknown as Event);
    http
      .expectOne((r) => r.url === '/api/organizations/org-b/my-time-off')
      .flush(page([{ ...item, id: 'new-team' }]));
    old.flush(page([item]));
    expect(fixture.componentInstance.items()[0].id).toBe('new-team');
  });
});
