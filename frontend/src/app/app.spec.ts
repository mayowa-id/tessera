import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { App } from './app';
describe('API connection', () => {
  let http: HttpTestingController;
  beforeEach(() => {
    TestBed.configureTestingModule({ imports: [App], providers: [provideHttpClient(), provideHttpClientTesting()] });
    http = TestBed.inject(HttpTestingController);
  });
  afterEach(() => http.verify());
  it('shows a connected database', () => {
    const fixture = TestBed.createComponent(App);
    http.expectOne('/api/readyz').flush({status:'ready'});
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('PostgreSQL is connected');
  });
  it('explains a database failure and permits retry', () => {
    const fixture = TestBed.createComponent(App);
    http.expectOne('/api/readyz').flush({}, {status:503, statusText:'Service Unavailable'});
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('PostgreSQL is unavailable');
    fixture.nativeElement.querySelector('button').click();
    http.expectOne('/api/readyz').flush({status:'ready'});
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('PostgreSQL is connected');
  });
});
