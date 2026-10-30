import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { vi } from 'vitest';
import { Demo } from './demo';

describe('Guest workspace', () => {
  let http: HttpTestingController;
  beforeEach(() => {
    TestBed.configureTestingModule({ providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()] });
    http = TestBed.inject(HttpTestingController);
  });
  afterEach(() => http.verify());

  it('lets visitors browse sample rosters and search people without accessing tenant APIs', () => {
    const fixture = TestBed.createComponent(Demo);
    const demo = fixture.componentInstance;
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('Illustrative sample');
    demo.view.set('rosters'); fixture.detectChanges();
    expect(fixture.nativeElement.querySelectorAll('.sample-shift').length).toBe(10);
    const original = demo.week(); demo.changeWeek(1);
    expect(demo.week()).not.toBe(original);
    demo.view.set('team'); demo.search.set('sam'); fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('tbody').textContent).toContain('Sam Taylor');
    expect(fixture.nativeElement.querySelector('tbody').textContent).not.toContain('Alex Morgan');
    http.expectNone(() => true);
  });

  it('prompts for signup when a visitor creates a roster, without sending a mutation', () => {
    const fixture = TestBed.createComponent(Demo);
    fixture.componentInstance.view.set('rosters'); fixture.detectChanges();
    const dialog = fixture.nativeElement.querySelector('dialog') as HTMLDialogElement;
    dialog.showModal = vi.fn();
    const create = Array.from(fixture.nativeElement.querySelectorAll('button') as NodeListOf<HTMLButtonElement>).find(button => button.textContent?.includes('Create roster'))!;
    create.click(); fixture.detectChanges();
    expect(dialog.showModal).toHaveBeenCalledOnce();
    expect(dialog.textContent).toContain('Sign up to create your own roster');
    expect(dialog.querySelector('a')?.getAttribute('href')).toBe('/signup');
    http.expectNone(() => true);
  });
});
