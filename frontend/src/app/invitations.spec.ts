import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { AuthService } from './auth.service';
import { Invitations } from './invitations';

describe('Team invitations',()=>{
 let http:HttpTestingController;
 beforeEach(()=>{
  TestBed.configureTestingModule({providers:[provideRouter([]),provideHttpClient(),provideHttpClientTesting()]});http=TestBed.inject(HttpTestingController);
  TestBed.inject(AuthService).identity.set({user:{id:'owner',email:'owner@example.com',display_name:'Owner'},memberships:[{org_id:'team',name:'Team',timezone:'Africa/Lagos',role:'admin'}],csrf_token:'token'});
 });
 afterEach(()=>http.verify());
 it('retains unsent teammates when a batch fails, then retries only those entries',async()=>{
  const component=TestBed.createComponent(Invitations).componentInstance;
  component.form.controls.batch.setValue('Alex, alex@example.com\nSam, sam@example.com');
  const pending=component.submit();http.expectOne('/api/organizations/team/invitations').flush({message:'Sent'});await Promise.resolve();
  const second=http.expectOne('/api/organizations/team/invitations');expect(second.request.body.email).toBe('sam@example.com');second.flush({message:'Delivery failed.'},{status:503,statusText:'Unavailable'});await pending;
  expect(component.form.controls.batch.value).toBe('Sam, sam@example.com');expect(component.message()).toBe('1 invitation(s) sent.');
  const retry=component.submit();const req=http.expectOne('/api/organizations/team/invitations');expect(req.request.body.email).toBe('sam@example.com');req.flush({message:'Sent'});await retry;
  expect(component.form.controls.batch.value).toBe('');
 });
 it('rejects duplicate email addresses before sending anything',async()=>{
  const component=TestBed.createComponent(Invitations).componentInstance;component.form.controls.batch.setValue('Alex, SAME@example.com\nSam, same@example.com');await component.submit();expect(component.error()).toContain('once');http.expectNone(()=>true);
 });
});
