import { TestBed } from '@angular/core/testing';
import { HttpClient, provideHttpClient, withInterceptors } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { AuthService, Identity, sessionInterceptor } from './auth.service';
import { Workspace } from './workspace';
import { Login } from './login';

const manager: Identity = {
  user: { id: 'user-a', email: 'manager@example.com', display_name: 'Alex Morgan' },
  memberships: [{ org_id: 'org-a', name: 'Team A', timezone: 'Africa/Lagos', role: 'manager' }],
  csrf_token: 'session-bound-token',
};

describe('Session access', () => {
  let auth: AuthService;
  let http: HttpTestingController;
  beforeEach(() => {
    TestBed.configureTestingModule({ providers: [provideRouter([]), provideHttpClient(withInterceptors([sessionInterceptor])), provideHttpClientTesting()] });
    auth = TestBed.inject(AuthService);
    http = TestBed.inject(HttpTestingController);
  });
  afterEach(() => http.verify());

  it('logs in and sends the session CSRF token on logout', () => {
    auth.login('manager@example.com', 'test password').subscribe();
    const login = http.expectOne('/api/auth/login');
    expect(login.request.headers.get('X-Tessera-Request')).toBe('1');
    login.flush(manager);
    expect(auth.identity()?.user.display_name).toBe('Alex Morgan');
    auth.logout().subscribe();
    const logout = http.expectOne('/api/auth/logout');
    expect(logout.request.headers.get('X-CSRF-Token')).toBe(manager.csrf_token);
    logout.flush(null);
    expect(auth.identity()).toBeNull();
  });

  it('clears an expired session', () => {
    auth.identity.set(manager);
    auth.restore().subscribe(value => expect(value).toBeNull());
    http.expectOne('/api/auth/me').flush({}, { status: 401, statusText: 'Unauthorized' });
    expect(auth.identity()).toBeNull();
  });

  it('does not send CSRF tokens to external origins', () => {
    auth.identity.set(manager);
    TestBed.inject(HttpClient).post('https://other.example/api/action', {}).subscribe();
    const request = http.expectOne('https://other.example/api/action');
    expect(request.request.headers.has('X-CSRF-Token')).toBe(false);
    request.flush({});
  });

  it('loads real organization counts for managers', () => {
    auth.identity.set(manager);
    const fixture = TestBed.createComponent(Workspace);
    http.expectOne('/api/organizations/org-a/summary').flush({ employees: 2, locations: 1, job_roles: 1 });
    fixture.detectChanges();
    expect(fixture.componentInstance.summary()?.employees).toBe(2);
    expect(fixture.nativeElement.textContent).toContain('Team A');
  });

  it('does not call manager endpoints for employees', () => {
    auth.identity.set({ ...manager, memberships: [{ ...manager.memberships[0], role: 'employee' }] });
    const fixture = TestBed.createComponent(Workspace);
    fixture.detectChanges();
    http.expectNone('/api/organizations/org-a/summary');
    expect(fixture.nativeElement.textContent).toContain('Your published shifts');
  });

  it('guides a new owner to a first roster and hides setup after publication', () => {
    auth.identity.set(manager);
    const fixture = TestBed.createComponent(Workspace);
    http.expectOne('/api/organizations/org-a/summary').flush({ employees: 1, locations: 1, job_roles: 1, rosters: 0, published_rosters: 0 });
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('Create your first roster');
    expect(fixture.nativeElement.textContent).toContain('Working solo?');
    fixture.componentInstance.loadSummary();
    http.expectOne('/api/organizations/org-a/summary').flush({ employees: 1, locations: 1, job_roles: 1, rosters: 1, published_rosters: 1 });
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('.first-week')).toBeNull();
    expect(fixture.nativeElement.textContent).toContain('Manage rosters');
  });

  it('validates the login form and displays rejected credentials', () => {
    const fixture = TestBed.createComponent(Login);
    http.expectOne('/api/auth/me').flush({}, { status: 401, statusText: 'Unauthorized' });
    fixture.componentInstance.submit();
    expect(fixture.componentInstance.error()).toContain('Enter your email');
    fixture.componentInstance.form.setValue({ email: 'manager@example.com', password: 'wrong password' });
    fixture.componentInstance.submit();
    http.expectOne('/api/auth/login').flush({ message: 'Email or password is incorrect.' }, { status: 401, statusText: 'Unauthorized' });
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('Email or password is incorrect.');
    expect(fixture.componentInstance.busy()).toBe(false);
  });

  it('opens without an error for a visitor with no session', () => {
    const fixture = TestBed.createComponent(Login);
    http.expectOne('/api/auth/me').flush({}, { status: 401, statusText: 'Unauthorized' });
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[role="alert"]')).toBeNull();
    expect(fixture.componentInstance.restoring()).toBe(false);
  });

  it('keeps a failed background session check separate from a failed sign-in', () => {
    const fixture = TestBed.createComponent(Login);
    http.expectOne('/api/auth/me').flush({}, { status: 503, statusText: 'Unavailable' });
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[role="alert"]')).toBeNull();
    expect(fixture.componentInstance.restoring()).toBe(false);
    fixture.componentInstance.form.setValue({ email: 'manager@example.com', password: 'test-password' });
    fixture.componentInstance.submit();
    http.expectOne('/api/auth/login').flush({}, { status: 503, statusText: 'Unavailable' });
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[role="alert"]')?.textContent).toContain('The service is unavailable');
  });
});
