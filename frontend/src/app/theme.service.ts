import { DOCUMENT } from '@angular/common';
import { DestroyRef, Injectable, inject, signal } from '@angular/core';

export type ThemePreference = 'light' | 'dark' | 'system';
export const THEME_STORAGE_KEY = 'tessera-theme';

@Injectable({ providedIn: 'root' })
export class ThemeService {
  private readonly document = inject(DOCUMENT);
  private readonly window = this.document.defaultView;
  private readonly system = this.window?.matchMedia?.('(prefers-color-scheme: dark)');
  readonly preference = signal<ThemePreference>(this.readPreference());

  constructor() {
    this.apply();
    const update = () => this.apply();
    this.system?.addEventListener('change', update);
    inject(DestroyRef).onDestroy(() => this.system?.removeEventListener('change', update));
  }

  setPreference(value: string): void {
    if (value !== 'light' && value !== 'dark' && value !== 'system') return;
    this.preference.set(value);
    try { this.window?.localStorage.setItem(THEME_STORAGE_KEY, value); } catch { /* Appearance still works without storage. */ }
    this.apply();
  }

  private readPreference(): ThemePreference {
    try {
      const value = this.window?.localStorage.getItem(THEME_STORAGE_KEY);
      if (value === 'light' || value === 'dark') return value;
    } catch { /* Use the device preference when storage is unavailable. */ }
    return 'system';
  }

  private apply(): void {
    const preference = this.preference();
    this.document.documentElement.dataset['theme'] = preference === 'system' ? (this.system?.matches ? 'dark' : 'light') : preference;
  }
}
