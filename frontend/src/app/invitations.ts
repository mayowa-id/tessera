import { Component, computed, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { AuthService, apiError } from './auth.service';

@Component({selector:'app-invitations',imports:[ReactiveFormsModule,RouterLink],template:`
 <main class="workspace-main"><a class="wordmark" routerLink="/workspace">tessera<span>.</span></a><p class="eyebrow">GROW TOGETHER</p><h1>Bring your people along.</h1><p class="muted">Invite teammates to {{organization()?.name}}. Each person receives a private link, valid for 7 days.</p>
 <label>Team <select [value]="selected()" (change)="select($event)">@for(org of organizations;track org.org_id){<option [value]="org.org_id">{{org.name}}</option>}</select></label>
 <form class="staff-editor" [formGroup]="form" (ngSubmit)="submit()"><label>Name <input formControlName="name" maxlength="120" required></label><label>Email <input type="email" formControlName="email" required></label><label>Access <select formControlName="role"><option value="employee">Employee</option>@if(organization()?.role==='admin'){<option value="manager">Manager</option>}</select></label><button class="primary" [disabled]="busy()||!organization()">{{busy()?'Sending…':'Send invitation'}}</button></form>
 @if(error()){<p class="error" role="alert">{{error()}}</p>}@if(message()){<p role="status">{{message()}}</p>}
 <p class="muted">Repeat for each teammate. Sending again replaces the previous link. Invited employees appear in Team & locations, where you can assign their job qualifications.</p><a routerLink="/staff" [queryParams]="{org:selected()}">Manage people and qualifications</a> · <a routerLink="/workspace">Back to workspace</a></main>`,styles:[`.workspace-main{max-width:1000px;margin:auto}.staff-editor{display:grid;gap:20px;margin:32px 0;max-width:500px}.staff-editor label{display:grid;gap:8px}.staff-editor input,.staff-editor select{padding:12px;font-size:16px}h1{font:normal 42px Georgia,serif}`]})
export class Invitations{
 private readonly http=inject(HttpClient);private readonly auth=inject(AuthService);private readonly route=inject(ActivatedRoute);
 readonly organizations=this.auth.identity()?.memberships.filter(m=>m.role==='admin'||m.role==='manager')??[];
 readonly selected=signal(this.organizations.find(m=>m.org_id===this.route.snapshot.queryParamMap.get('org'))?.org_id??this.organizations[0]?.org_id??'');
 readonly organization=computed(()=>this.organizations.find(m=>m.org_id===this.selected()));
 readonly busy=signal(false);readonly error=signal('');readonly message=signal('');
 readonly form=inject(FormBuilder).nonNullable.group({name:['',[Validators.required,Validators.maxLength(120)]],email:['',[Validators.required,Validators.email]],role:['employee']});
 select(event:Event){this.selected.set((event.target as HTMLSelectElement).value);this.form.controls.role.setValue('employee');this.message.set('');}
 submit(){if(this.busy())return;if(this.form.invalid){this.error.set('Enter a name and valid email address.');return;}this.busy.set(true);this.error.set('');this.message.set('');this.http.post<{message:string}>('/api/organizations/'+this.selected()+'/invitations',this.form.getRawValue()).subscribe({next:r=>{this.busy.set(false);this.message.set(r.message);this.form.reset({name:'',email:'',role:'employee'});},error:e=>{this.busy.set(false);this.error.set(apiError(e));}});}
}
