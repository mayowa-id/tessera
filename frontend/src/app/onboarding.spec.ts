import { TestBed } from '@angular/core/testing';
import { provideHttpClient, withInterceptors } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ActivatedRoute, provideRouter } from '@angular/router';
import { Onboarding } from './onboarding';
import { Welcome } from './welcome';
import { sessionInterceptor } from './auth.service';

describe('Public onboarding',()=>{
 let http:HttpTestingController;
 function setup(mode='signup'){
  TestBed.configureTestingModule({providers:[provideRouter([]),provideHttpClient(withInterceptors([sessionInterceptor])),provideHttpClientTesting(),{provide:ActivatedRoute,useValue:{snapshot:{data:{mode}}}}]});
  http=TestBed.inject(HttpTestingController);
 }
 afterEach(()=>{http?.verify();window.history.replaceState(null,'','/');});
 it('shows a public illustrative roster without fetching identity or real tenant data',()=>{
  setup();const fixture=TestBed.createComponent(Welcome);fixture.detectChanges();
  expect(fixture.nativeElement.textContent).toContain('Illustrative sample');
  expect(fixture.nativeElement.querySelector('a[href="/signup"]')).toBeTruthy();
  http.expectNone(()=>true);
 });
 it('creates a team and waits for email verification without establishing a session',()=>{
  setup();const fixture=TestBed.createComponent(Onboarding);const component=fixture.componentInstance;
  component.submit();http.expectNone('/api/auth/signup');
  component.form.setValue({email:'owner@example.com',password:'strong-password',name:'Owner',team:'Team',location:'Main site',timezone:'Africa/Lagos'});
  component.submit();const req=http.expectOne('/api/auth/signup');expect(req.request.headers.get('X-Tessera-Request')).toBe('1');expect(req.request.body.team).toBe('Team');
  req.flush({message:'Check your email.'});expect(component.complete()).toBe(true);expect(component.form.controls.password.value).toBe('');
 });
 it('submits invitation tokens only in a POST and removes the token from the address bar',()=>{
  window.history.replaceState(null,'','/join#'+ 'a'.repeat(43));setup('join');const fixture=TestBed.createComponent(Onboarding);
  expect(window.location.hash).toBe('');fixture.componentInstance.form.controls.password.setValue('existing-password');fixture.componentInstance.submit();
  const req=http.expectOne('/api/auth/join');expect(req.request.body.token).toBe('a'.repeat(43));req.flush({message:'Joined.'});expect(fixture.componentInstance.complete()).toBe(true);
 });
 it('keeps the form retryable when delivery fails',()=>{
  setup('resend');const fixture=TestBed.createComponent(Onboarding);fixture.componentInstance.form.controls.email.setValue('owner@example.com');fixture.componentInstance.submit();
  http.expectOne('/api/auth/resend').flush({message:'Delivery failed.'},{status:503,statusText:'Unavailable'});
  expect(fixture.componentInstance.complete()).toBe(false);expect(fixture.componentInstance.error()).toBe('Delivery failed.');expect(fixture.componentInstance.busy()).toBe(false);
 });
});
