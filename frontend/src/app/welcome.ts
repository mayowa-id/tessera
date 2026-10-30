import { Component } from '@angular/core';
import { RouterLink } from '@angular/router';

@Component({
 selector: 'app-welcome', imports: [RouterLink],
 template: `
 <main class="welcome">
 <header><a class="wordmark" routerLink="/">tessera<span>.</span></a><nav><a routerLink="/login">Sign in</a><a class="primary" routerLink="/signup">Sign up</a></nav></header>
 <section class="welcome-intro"><p class="eyebrow">EVERY SHIFT IN ITS PLACE</p><h1>A good week starts<br>with a <em>plan.</em></h1><p>A shared space for your people, their time, and the work ahead.</p><a class="primary" routerLink="/demo">Explore the app ↗</a><a class="text-link" href="#sample">Explore a sample week ↓</a></section>
 <section id="sample" aria-labelledby="sample-title"><div class="directory-heading"><div><p class="eyebrow">A LITTLE LOOK AHEAD</p><h2 id="sample-title">Your week, at a glance.</h2><p class="muted">Illustrative sample · Main site · No real team data</p></div><a class="primary" routerLink="/signup">Build your own roster</a></div>
 <div class="table-scroll"><table class="directory-table"><caption>Sample roster — Monday to Friday</caption><thead><tr><th>Team member</th><th>Monday</th><th>Tuesday</th><th>Wednesday</th><th>Thursday</th><th>Friday</th></tr></thead><tbody>
 <tr><th>Alex Morgan</th><td>09:00–17:00</td><td>09:00–17:00</td><td>Off</td><td>09:00–17:00</td><td>09:00–17:00</td></tr>
 <tr><th>Sam Taylor</th><td>Off</td><td>10:00–18:00</td><td>10:00–18:00</td><td>Off</td><td>10:00–18:00</td></tr>
 <tr><th>Jordan Lee</th><td>08:00–16:00</td><td>Off</td><td>08:00–16:00</td><td>08:00–16:00</td><td>Off</td></tr>
 </tbody></table></div><p class="muted">Explore freely. Create a verified account when you are ready to save your own team and shifts.</p></section>
 <footer><span>Built thoughtfully, one shift at a time.</span><a routerLink="/signup">Bring your team along ↗</a></footer>
 </main>`,
 styles: [`:host{display:block}.welcome{max-width:1200px;margin:auto;padding:32px}.welcome header,.welcome nav,.welcome footer{display:flex;align-items:center;justify-content:space-between;gap:24px}.welcome nav a{text-decoration:none}.welcome-intro{padding:90px 0}.welcome h1{font:normal clamp(48px,7vw,88px)/1.05 Georgia,serif;letter-spacing:-3px;margin:22px 0}.welcome h1 em{color:var(--green)}.welcome-intro p{line-height:1.8;margin-bottom:32px}.welcome-intro>a{display:inline-block;margin:8px 24px 8px 0}.primary{text-decoration:none}.welcome footer{padding:60px 0 0;font-size:12px;color:var(--muted)}caption{text-align:left;padding:16px;font-size:12px}@media(max-width:600px){.welcome{padding:20px}.welcome header,.directory-heading{align-items:start;flex-direction:column}.welcome-intro{padding:55px 0}}`],
})
export class Welcome {}
