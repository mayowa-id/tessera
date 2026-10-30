import { Routes } from '@angular/router';
import { managerGuard, signedInGuard } from './auth.service';

export const routes: Routes = [
  { path: '', pathMatch: 'full', loadComponent: () => import('./welcome').then(module => module.Welcome) },
  ...['signup', 'verify-email', 'resend', 'forgot-password', 'reset-password', 'join'].map(mode => ({ path: mode, data: { mode }, loadComponent: () => import('./onboarding').then(module => module.Onboarding) })),
  { path: 'invitations', canActivate: [managerGuard], loadComponent: () => import('./invitations').then(module => module.Invitations) },
  { path: 'login', loadComponent: () => import('./login').then(module => module.Login) },
  { path: 'workspace', canActivate: [signedInGuard], loadComponent: () => import('./workspace').then(module => module.Workspace) },
  { path: 'staff', canActivate: [managerGuard], loadComponent: () => import('./staff').then(module => module.Staff) },
  { path: 'rosters', canActivate: [managerGuard], loadComponent: () => import('./rosters').then(module => module.Rosters) },
  { path: 'system', loadComponent: () => import('./foundation').then(module => module.Foundation) },
  { path: '**', redirectTo: 'workspace' },
];
