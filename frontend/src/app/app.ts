import { Component, inject } from '@angular/core';
import { RouterOutlet } from '@angular/router';
import { ThemeService } from './theme.service';
import { Icon } from './icon';
@Component({ selector: 'app-root', imports: [RouterOutlet, Icon], template: `
  <div class="appearance-bar"><div class="theme-switch" role="group" aria-label="Appearance">
    <button type="button" aria-label="Light mode" title="Light mode" [attr.aria-pressed]="theme.preference() === 'light'" (click)="theme.setPreference('light')"><app-icon name="sun" /></button>
    <button type="button" aria-label="Dark mode" title="Dark mode" [attr.aria-pressed]="theme.preference() === 'dark'" (click)="theme.setPreference('dark')"><app-icon name="moon" /></button>
    <button type="button" aria-label="Use device theme" title="Use device theme" [attr.aria-pressed]="theme.preference() === 'system'" (click)="theme.setPreference('system')"><app-icon name="monitor" /></button>
  </div>
  </div>
  <router-outlet />
` })
export class App { readonly theme = inject(ThemeService); }
