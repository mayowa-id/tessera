import { Component, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { apiError } from './auth.service';

@Component({
 selector:'app-onboarding', imports:[ReactiveFormsModule,RouterLink],
 template:`<main class="onboarding-page"><a class="wordmark" routerLink="/">tessera<span>.</span></a><section class="onboarding-card"><p class="eyebrow">A PLACE FOR YOUR TEAM</p><h1>{{ title }}</h1><p class="muted">{{ description }}</p>
 <form [formGroup]="form" (ngSubmit)="submit()">
 @if(mode==='signup') { <label>Your name<input formControlName="name" autocomplete="name" maxlength="120" required></label><label>Team name<input formControlName="team" autocomplete="organization" maxlength="120" required></label><label>First location<input formControlName="location" maxlength="120" required></label><label>Team timezone<input formControlName="timezone" placeholder="Africa/Lagos" required></label> }
 @if(mode==='signup'||mode==='resend'||mode==='forgot-password') { <label>Email address<input formControlName="email" type="email" autocomplete="email" required></label> }
 @if(mode==='signup'||mode==='verify-email'||mode==='reset-password'||mode==='join') { <label>{{ mode==='join' ? 'Existing password, or choose one for a new account' : mode==='verify-email' ? 'Your account password' : 'Choose a password' }}<input formControlName="password" type="password" [attr.autocomplete]="mode==='verify-email'||mode==='join'?'current-password':'new-password'" minlength="12" maxlength="72" required></label><small>12–72 bytes. Use a unique password.</small> }
 @if(error()){<p class="error" role="alert">{{error()}}</p>}@if(message()){<p role="status">{{message()}}</p>}
 <button class="primary" [disabled]="busy()||complete()">{{busy()?'Please wait…':action}}</button></form>
 <p><a routerLink="/login">Sign in</a> · <a routerLink="/demo">Keep exploring</a></p><p><a routerLink="/resend">Request a verification email</a> · <a routerLink="/forgot-password">Reset password</a></p>
 </section></main>`,
 styles:[`.onboarding-page{max-width:760px;margin:auto;padding:36px 24px}.onboarding-card{max-width:520px;margin:56px auto}.onboarding-card h1{font:normal 42px Georgia,serif}.onboarding-card form,.onboarding-card label{display:grid;gap:10px}.onboarding-card form{gap:20px;margin:30px 0}.onboarding-card input{width:100%;padding:13px;border:1px solid var(--border);border-radius:6px;background:var(--surface);color:var(--heading);font-size:16px}.onboarding-card label,.onboarding-card p,.onboarding-card small{font-size:13px;line-height:1.7}.primary:disabled{opacity:.6}`],
})
export class Onboarding {
 private readonly http=inject(HttpClient);
 private readonly route=inject(ActivatedRoute);
 readonly mode=this.route.snapshot.data['mode'] as string;
 readonly busy=signal(false); readonly complete=signal(false); readonly error=signal(''); readonly message=signal('');
 private readonly token=window.location.hash.slice(1);
 readonly form=inject(FormBuilder).nonNullable.group({name:[''],team:[''],location:['Main site'],timezone:[Intl.DateTimeFormat().resolvedOptions().timeZone||'Africa/Lagos'],email:[''],password:['']});
 readonly title=({signup:'Create your account.','verify-email':'Verify your email.',resend:'A fresh verification link.','forgot-password':'Find your way back.','reset-password':'Choose a new password.',join:'Join your team.'} as Record<string,string>)[this.mode];
 readonly description=this.mode==='signup'?'Create your team and first location. Verify your email, then invite your people.':this.mode==='join'?'Your invitation links your account to the team. Existing members must use their current password.':this.mode==='verify-email'?'Enter the password you chose when creating your team.':'Follow the steps to securely access your account.';
 readonly action=this.mode==='signup'?'Sign up & create a team':this.mode==='join'?'Join team':this.mode==='verify-email'?'Verify email':this.mode==='reset-password'?'Reset password':'Send email';
 constructor(){
  // Link secrets stay out of request URLs, server logs and navigation history.
  if(this.token)window.history.replaceState(null,'',window.location.pathname);
  const c=this.form.controls;
  if(this.mode==='signup'){for(const field of [c.name,c.team,c.location,c.timezone])field.setValidators(Validators.required);}
  if(['signup','resend','forgot-password'].includes(this.mode))c.email.setValidators([Validators.required,Validators.email]);
  if(['signup','verify-email','reset-password','join'].includes(this.mode))c.password.setValidators([Validators.required,Validators.minLength(12),Validators.maxLength(72)]);
  for(const field of Object.values(c))field.updateValueAndValidity();
 }
 submit(){
  if(this.busy()||this.complete())return;if(this.form.invalid){this.error.set('Complete the required fields.');return;}
  const value=this.form.getRawValue();let body:object;let endpoint:string;
  if(this.mode==='signup'){body=value;endpoint='signup';}
  else if(this.mode==='resend'||this.mode==='forgot-password'){body={email:value.email};endpoint=this.mode;}
  else {if(!this.token){this.error.set('Open the complete link from your email.');return;}body={token:this.token,password:value.password};endpoint=this.mode==='verify-email'?'verify':this.mode;}
  this.busy.set(true);this.error.set('');
  this.http.post<{message:string}>('/api/auth/'+endpoint,body).subscribe({next:response=>{this.message.set(response.message);this.busy.set(false);this.complete.set(true);this.form.controls.password.reset();},error:error=>{this.error.set(apiError(error));this.busy.set(false);}});
 }
}
