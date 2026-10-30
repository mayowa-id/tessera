import { Component, computed, inject, signal } from '@angular/core';
import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { AuthService, apiError } from './auth.service';

interface Summary { employees: number; locations: number; job_roles: number; }

@Component({
  selector: 'app-workspace',
  imports: [RouterLink],
  template: `
    <div class="workspace">
      <aside class="sidebar"><a class="wordmark" routerLink="/workspace">tessera<span>.</span></a><p class="eyebrow">YOUR WORKSPACE</p><span class="nav-current">Overview</span>@if (isManager()) { <a routerLink="/staff" [queryParams]="{ org: selected() }">Team & locations</a><a routerLink="/rosters" [queryParams]="{ org: selected() }">Rosters</a> }<a routerLink="/system">System connection</a><div class="sidebar-bottom"><span>{{ auth.identity()?.user?.display_name }}</span><small>{{ membership()?.role }}</small><button class="secondary" (click)="logout()" [disabled]="signingOut()">{{ signingOut() ? 'Signing out...' : 'Sign out' }}</button></div></aside>
      <main class="workspace-main">
        <header class="workspace-header"><span class="eyebrow">TEAM OVERVIEW</span>@if (memberships().length > 1) { <label>Organization <select [value]="selected()" (change)="selectOrganization($event)">@for (org of memberships(); track org.org_id) { <option [value]="org.org_id">{{ org.name }}</option> }</select></label> } @else { <span class="organization-name">{{ membership()?.name }}</span> }</header>
        <p class="eyebrow greeting">A PLACE FOR YOUR TEAM</p><h1>Welcome, {{ firstName() }}<span class="accent">.</span></h1><p class="muted">{{ membership()?.name || 'Your workspace' }} · {{ membership()?.timezone || 'Organization access' }}</p>
        @if (error()) { <p class="error" role="alert">{{ error() }}</p><button class="secondary" (click)="loadSummary()">Try again</button> }
        @if (!membership()) { <section class="workspace-card"><h2>No organization access yet</h2><p>Ask your manager to add your account to an organization.</p></section> }
        @else if (isManager()) {
          <div class="stat-grid">@for (stat of stats(); track stat.label) { <section class="stat"><span>{{ stat.label }}</span><strong>{{ loading() ? '...' : stat.value }}</strong></section> }</div>
          <section class="workspace-card"><p class="eyebrow">YOUR NEXT CHAPTER</p><h2>Your team starts here.</h2><p>Manage your team, locations, and job qualifications. Build weekly drafts and assign shifts with scheduling checks.</p><a class="text-link" routerLink="/rosters" [queryParams]="{ org: selected() }">Build a roster</a> · <a class="text-link" routerLink="/staff" [queryParams]="{ org: selected() }">Manage your team</a><span class="status-note">FOUNDATION · ACCESS & IDENTITY</span></section>
        } @else { <section class="workspace-card"><p class="eyebrow">YOUR SCHEDULE</p><h2>A clearer week is on its way.</h2><p>You're signed in to your team's workspace. Your published shifts will appear here once scheduling is available.</p><span class="status-note">SCHEDULING COMING NEXT</span></section> }
      </main>
    </div>
  `,
})
export class Workspace {
  readonly auth = inject(AuthService);
  private readonly http = inject(HttpClient);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);
  readonly memberships = computed(() => this.auth.identity()?.memberships ?? []);
  readonly selected = signal(this.memberships().find(org => org.org_id === this.route.snapshot.queryParamMap.get('org'))?.org_id ?? this.memberships()[0]?.org_id ?? '');
  readonly membership = computed(() => this.memberships().find(org => org.org_id === this.selected()));
  readonly isManager = computed(() => ['admin', 'manager'].includes(this.membership()?.role ?? ''));
  readonly firstName = computed(() => this.auth.identity()?.user.display_name.split(' ')[0] ?? 'there');
  readonly summary = signal<Summary | null>(null);
  readonly loading = signal(false);
  readonly signingOut = signal(false);
  readonly error = signal('');
  readonly stats = computed(() => [
    { label: 'Team members', value: this.summary()?.employees ?? '—' },
    { label: 'Locations', value: this.summary()?.locations ?? '—' },
    { label: 'Job roles', value: this.summary()?.job_roles ?? '—' },
  ]);

  constructor() { this.loadSummary(); }
  selectOrganization(event: Event): void { this.selected.set((event.target as HTMLSelectElement).value); this.loadSummary(); }
  loadSummary(): void {
    this.summary.set(null); this.error.set('');
    if (!this.isManager()) return;
    const orgID = this.selected();
    this.loading.set(true);
    this.http.get<Summary>('/api/organizations/' + encodeURIComponent(orgID) + '/summary').subscribe({
      next: summary => { if (orgID === this.selected()) { this.summary.set(summary); this.loading.set(false); } },
      error: error => { if (orgID === this.selected()) { this.loading.set(false); this.handleError(error); } },
    });
  }
  logout(): void {
    this.signingOut.set(true); this.error.set('');
    this.auth.logout().subscribe({
      next: () => { void this.router.navigateByUrl('/login'); },
      error: error => { this.signingOut.set(false); this.handleError(error); },
    });
  }
  private handleError(error: unknown): void {
    if (error instanceof HttpErrorResponse && error.status === 401) { this.auth.identity.set(null); void this.router.navigateByUrl('/login'); return; }
    this.error.set(apiError(error));
  }
}
