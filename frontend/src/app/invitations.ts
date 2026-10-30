import { Component, computed, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { FormBuilder, FormControl, ReactiveFormsModule, Validators } from '@angular/forms';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import { AuthService, apiError } from './auth.service';

@Component({selector:'app-invitations',imports:[ReactiveFormsModule,RouterLink],template:`
 <main class="workspace-main"><a class="wordmark" routerLink="/workspace">tessera<span>.</span></a><p class="eyebrow">GROW TOGETHER</p><h1>Bring your people along.</h1><p class="muted">Invite teammates to {{organization()?.name}}. Each person receives a private link, valid for 7 days.</p>
 <label>Team <select [value]="selected()" (change)="select($event)">@for(org of organizations;track org.org_id){<option [value]="org.org_id">{{org.name}}</option>}</select></label>
 <form class="staff-editor" [formGroup]="form" (ngSubmit)="submit()"><label>Name <input formControlName="name" maxlength="120"></label><label>Email <input type="email" formControlName="email"></label><label>Or invite several teammates (up to 10)<textarea formControlName="batch" rows="5" placeholder="Alex Morgan, alex@example.com&#10;Sam Taylor, sam@example.com"></textarea><small>One name and email per line, separated by a comma. This replaces the individual fields above.</small></label><label>Access <select formControlName="role"><option value="employee">Employee</option>@if(organization()?.role==='admin'){<option value="manager">Manager</option>}</select></label><button class="primary" [disabled]="busy()||!organization()">{{busy()?'Sending…':'Send invitations'}}</button></form>
 @if(error()){<p class="error" role="alert">{{error()}}</p>}@if(message()){<p role="status">{{message()}}</p>}
 <p class="muted">Sending again replaces the previous link. Invited employees appear in Team & locations, where you can assign their job qualifications.</p><a routerLink="/staff" [queryParams]="{org:selected()}">Manage people and qualifications</a> · <a routerLink="/workspace">Back to workspace</a></main>`,styles:[`.workspace-main{max-width:1000px;margin:auto}.staff-editor{display:grid;gap:20px;margin:32px 0;max-width:500px}.staff-editor label{display:grid;gap:8px}.staff-editor input,.staff-editor select,.staff-editor textarea{padding:12px;font-size:16px}h1{font:normal 42px Georgia,serif}`]})
export class Invitations{
 private readonly http=inject(HttpClient);private readonly auth=inject(AuthService);private readonly route=inject(ActivatedRoute);
 readonly organizations=this.auth.identity()?.memberships.filter(m=>m.role==='admin'||m.role==='manager')??[];
 readonly selected=signal(this.organizations.find(m=>m.org_id===this.route.snapshot.queryParamMap.get('org'))?.org_id??this.organizations[0]?.org_id??'');
 readonly organization=computed(()=>this.organizations.find(m=>m.org_id===this.selected()));
 readonly busy=signal(false);readonly error=signal('');readonly message=signal('');
 readonly form=inject(FormBuilder).nonNullable.group({name:[''],email:[''],batch:[''],role:['employee']});
 select(event:Event){this.selected.set((event.target as HTMLSelectElement).value);this.form.controls.role.setValue('employee');this.message.set('');}
 async submit(){
  if(this.busy()||!this.organization())return;
  const {name,email,batch,role}=this.form.getRawValue();
  const lines=batch.trim()?batch.split('\n').map(line=>line.trim()).filter(Boolean):[];
  const people=lines.length?lines.map(line=>{const parts=line.split(',');return {name:parts[0]?.trim()??'',email:parts.length===2?parts[1].trim():''};}):[{name:name.trim(),email:email.trim()}];
  if(people.length>10||people.some(p=>!p.name||p.name.length>120||!p.email||Validators.email(new FormControl(p.email)))){this.error.set('Enter up to 10 people with a name and valid email address. Use Name, email for each line.');return;}
  if(new Set(people.map(p=>p.email.toLowerCase())).size!==people.length){this.error.set('Each email address should appear once.');return;}
  this.busy.set(true);this.error.set('');this.message.set('');const org=this.selected();let sent=0;
  try{for(const person of people){await firstValueFrom(this.http.post('/api/organizations/'+org+'/invitations',{...person,role}));sent++;this.message.set(sent+' invitation(s) sent.');if(lines.length)this.form.controls.batch.setValue(lines.slice(sent).join('\n'));}if(!lines.length)this.form.patchValue({name:'',email:''});}
  catch(error){this.error.set(apiError(error)+' '+sent+' invitation(s) sent; remaining entries are kept for retry.');}
  finally{this.busy.set(false);}
 }
}
