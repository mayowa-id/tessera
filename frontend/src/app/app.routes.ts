import { Routes } from '@angular/router';
import { managerGuard, signedInGuard } from './auth.service';

export const routes: Routes = [
  { path: '', pathMatch: 'full', redirectTo: 'workspace' },
  { path: 'login', loadComponent: () => import('./login').then(module => module.Login) },
  { path: 'workspace', canActivate: [signedInGuard], loadComponent: () => import('./workspace').then(module => module.Workspace) },
  { path: 'staff', canActivate: [managerGuard], loadComponent: () => import('./staff').then(module => module.Staff) },
  { path: 'rosters', canActivate: [managerGuard], loadComponent: () => import('./rosters').then(module => module.Rosters) },
  { path: 'my-shifts', canActivate: [signedInGuard], loadComponent: () => import('./my-shifts').then(module => module.MyShifts) },
  { path: 'activity', canActivate: [managerGuard], loadComponent: () => import('./activity').then(module => module.Activity) },
  { path: 'system', loadComponent: () => import('./foundation').then(module => module.Foundation) },
  { path: '**', redirectTo: 'workspace' },
];
