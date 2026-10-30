import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';
import { ThemeService, THEME_STORAGE_KEY } from './theme.service';

describe('Appearance preferences', () => {
  let dark: boolean;
  let changed: (() => void) | undefined;
  beforeEach(() => {
    localStorage.removeItem(THEME_STORAGE_KEY);
    dark = false;
    changed = undefined;
    vi.stubGlobal('matchMedia', () => ({
      get matches() { return dark; },
      addEventListener: (_event: string, listener: () => void) => { changed = listener; },
      removeEventListener: vi.fn(),
    }));
    TestBed.configureTestingModule({});
  });
  afterEach(() => {
    TestBed.resetTestingModule();
    localStorage.removeItem(THEME_STORAGE_KEY);
    delete document.documentElement.dataset['theme'];
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('follows device changes until a specific theme is selected', () => {
    const theme = TestBed.inject(ThemeService);
    expect(document.documentElement.dataset['theme']).toBe('light');
    dark = true; changed?.();
    expect(document.documentElement.dataset['theme']).toBe('dark');
    theme.setPreference('light'); changed?.();
    expect(document.documentElement.dataset['theme']).toBe('light');
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('light');
    theme.setPreference('system');
    expect(document.documentElement.dataset['theme']).toBe('dark');
  });

  it('restores the saved theme instead of the device default', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'dark');
    const theme = TestBed.inject(ThemeService);
    expect(theme.preference()).toBe('dark');
    expect(document.documentElement.dataset['theme']).toBe('dark');
  });

  it('still allows switching when browser storage is unavailable', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('Blocked'); });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('Blocked'); });
    const theme = TestBed.inject(ThemeService);
    theme.setPreference('dark');
    expect(document.documentElement.dataset['theme']).toBe('dark');
  });
});
