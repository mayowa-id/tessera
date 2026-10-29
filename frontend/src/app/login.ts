import { Component, inject, signal } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { AuthService, apiError } from './auth.service';

@Component({
  selector: 'app-login',
  imports: [ReactiveFormsModule, RouterLink],
  template: `
    <main class="login-page">
      <section class="login-story">
        <a class="wordmark" routerLink="/login">tessera<span>.</span></a>
        <div><p class="eyebrow">EVERY SHIFT IN ITS PLACE</p><h1>A good week<br>starts with<br>a <em>plan.</em></h1><p class="story-copy">A shared space for your people,<br>their time, and the work ahead.</p></div>
        <p class="story-footer">Built thoughtfully, one shift at a time.</p>
      </section>
      <section class="login-form" aria-labelledby="login-title">
        <p class="eyebrow">YOUR TEAM'S WORKSPACE</p><h2 id="login-title">Welcome back.</h2><p class="muted">Sign in to see where the week takes you.</p>
        <form [formGroup]="form" (ngSubmit)="submit()">
          <label for="email">Email address</label><input id="email" type="email" autocomplete="username" formControlName="email" placeholder="you@yourteam.com" [attr.aria-invalid]="form.controls.email.touched && form.controls.email.invalid">
          <label for="password">Password</label><input id="password" type="password" autocomplete="current-password" formControlName="password" placeholder="Your password" [attr.aria-invalid]="form.controls.password.touched && form.controls.password.invalid">
          @if (error()) { <p class="error" role="alert">{{ error() }}</p> }
          <button type="submit" [disabled]="busy() || restoring()">{{ busy() ? 'Signing in...' : restoring() ? 'Checking session...' : 'Sign in' }} <span aria-hidden="true">↗</span></button>
        </form>
        <p class="form-note">Your organization provides your account. Ask your manager if you need access.</p>
        <a class="text-link" routerLink="/system">Check system connection</a>
      </section>
    </main>
  `,
})
export class Login {
  private readonly auth = inject(AuthService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);
  readonly busy = signal(false);
  readonly restoring = signal(true);
  readonly error = signal('');
  readonly form = inject(FormBuilder).nonNullable.group({
    email: ['', [Validators.required, Validators.email, Validators.maxLength(254)]],
    password: ['', [Validators.required]],
  });

  constructor() {
    if (this.route.snapshot.queryParamMap.get('reason') === 'unavailable') this.error.set('The service is unavailable. Please try again.');
    this.auth.restore().subscribe({
      next: identity => { this.restoring.set(false); if (identity) void this.router.navigateByUrl('/workspace'); },
      error: error => { this.restoring.set(false); this.error.set(apiError(error)); },
    });
  }

  submit(): void {
    if (this.busy() || this.restoring()) return;
    if (this.form.invalid) { this.form.markAllAsTouched(); this.error.set('Enter your email address and password.'); return; }
    this.error.set(''); this.busy.set(true);
    const { email, password } = this.form.getRawValue();
    this.auth.login(email, password).subscribe({
      next: () => { this.busy.set(false); this.form.controls.password.reset(); void this.router.navigateByUrl('/workspace'); },
      error: error => { this.busy.set(false); this.error.set(apiError(error)); },
    });
  }
}
