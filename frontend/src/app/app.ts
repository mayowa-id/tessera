import { Component, inject } from '@angular/core';
import { RouterOutlet } from '@angular/router';
import { ThemeService } from './theme.service';
@Component({ selector: 'app-root', imports: [RouterOutlet], template: `
  <div class="appearance-bar"><label for="theme-preference">Appearance</label>
    <select id="theme-preference" [value]="theme.preference()" (change)="theme.setPreference($any($event.target).value)">
      <option value="light">Light</option><option value="dark">Dark</option><option value="system">Use device setting</option>
    </select>
  </div>
  <router-outlet />
` })
export class App { readonly theme = inject(ThemeService); }
