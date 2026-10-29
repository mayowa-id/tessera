import { Injectable, inject, signal } from '@angular/core';
import { HttpClient, HttpErrorResponse, HttpInterceptorFn } from '@angular/common/http';
import { CanActivateFn, Router } from '@angular/router';
import { catchError, map, of, tap, throwError } from 'rxjs';

export interface Membership { org_id: string; name: string; timezone: string; role: 'admin' | 'manager' | 'employee'; }
export interface Identity {
  user: { id: string; email: string; display_name: string };
  memberships: Membership[];
  csrf_token: string;
}

@Injectable({ providedIn: 'root' })
export class AuthService {
  private readonly http = inject(HttpClient);
  readonly identity = signal<Identity | null>(null);

  restore() {
    return this.http.get<Identity>('/api/auth/me').pipe(
      tap(identity => this.identity.set(identity)),
      catchError((error: HttpErrorResponse) => {
        if (error.status === 401) { this.identity.set(null); return of(null); }
        return throwError(() => error);
      }),
    );
  }
  login(email: string, password: string) {
    return this.http.post<Identity>('/api/auth/login', { email, password }).pipe(tap(identity => this.identity.set(identity)));
  }
  logout() {
    return this.http.post<void>('/api/auth/logout', {}).pipe(tap(() => this.identity.set(null)));
  }
}

export const sessionInterceptor: HttpInterceptorFn = (request, next) => {
  const auth = inject(AuthService);
  if (request.url.startsWith('/api/') && !['GET', 'HEAD', 'OPTIONS'].includes(request.method)) {
    const headers: Record<string, string> = { 'X-Tessera-Request': '1' };
    const token = auth.identity()?.csrf_token;
    if (token) headers['X-CSRF-Token'] = token;
    request = request.clone({ setHeaders: headers });
  }
  return next(request);
};

export const signedInGuard: CanActivateFn = () => {
  const auth = inject(AuthService);
  const router = inject(Router);
  return auth.restore().pipe(
    map(identity => identity ? true : router.createUrlTree(['/login'])),
    catchError(() => of(router.createUrlTree(['/login'], { queryParams: { reason: 'unavailable' } }))),
  );
};

export const managerGuard: CanActivateFn = () => {
  const auth = inject(AuthService);
  const router = inject(Router);
  return auth.restore().pipe(
    map(identity => !identity ? router.createUrlTree(['/login']) : identity.memberships.some(org => org.role === 'admin' || org.role === 'manager') ? true : router.createUrlTree(['/workspace'])),
    catchError(() => of(router.createUrlTree(['/login'], { queryParams: { reason: 'unavailable' } }))),
  );
};

export function apiError(error: unknown): string {
  if (error instanceof HttpErrorResponse) {
    if (error.status === 0) return 'Unable to reach the API. Check your connection and try again.';
    if (typeof error.error?.message === 'string') return error.error.message;
  }
  return 'Something went wrong. Please try again.';
}
