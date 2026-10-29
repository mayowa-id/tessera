import { Routes } from '@angular/router';
import { signedInGuard } from './auth.service';

export const routes: Routes = [
  { path: '', pathMatch: 'full', redirectTo: 'workspace' },
  { path: 'login', loadComponent: () => import('./login').then(module => module.Login) },
  { path: 'workspace', canActivate: [signedInGuard], loadComponent: () => import('./workspace').then(module => module.Workspace) },
  { path: 'system', loadComponent: () => import('./foundation').then(module => module.Foundation) },
  { path: '**', redirectTo: 'workspace' },
];
