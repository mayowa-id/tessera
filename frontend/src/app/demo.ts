import { Component, ElementRef, computed, inject, signal, viewChild } from '@angular/core';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { localMonday } from './roster-time';

type DemoView = 'overview' | 'rosters' | 'team';
interface SamplePerson { name: string; initials: string; role: string; hours: number; days: (string | null)[]; }

@Component({ selector: 'app-demo', imports: [RouterLink], templateUrl: './demo.html', styleUrl: './demo.css' })
export class Demo {
  private readonly route = inject(ActivatedRoute);
  private readonly dialog = viewChild.required<ElementRef<HTMLDialogElement>>('signupDialog');
  readonly view = signal<DemoView>(this.initialView());
  readonly week = signal(localMonday('Africa/Lagos'));
  readonly search = signal('');
  readonly action = signal('save your own roster');
  readonly selectedShift = signal<{ name: string; day: string; hours: string } | null>(null);
  readonly people: SamplePerson[] = [
    { name: 'Alex Morgan', initials: 'AM', role: 'Team lead', hours: 32, days: ['09:00–17:00', '09:00–17:00', null, '09:00–17:00', '09:00–17:00'] },
    { name: 'Sam Taylor', initials: 'ST', role: 'Team member', hours: 24, days: [null, '10:00–18:00', '10:00–18:00', null, '10:00–18:00'] },
    { name: 'Jordan Lee', initials: 'JL', role: 'Team member', hours: 24, days: ['08:00–16:00', null, '08:00–16:00', '08:00–16:00', null] },
  ];
  readonly filteredPeople = computed(() => this.people.filter(person => person.name.toLowerCase().includes(this.search().toLowerCase())));
  readonly days = ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday'];
  readonly weekLabel = computed(() => new Intl.DateTimeFormat('en-GB', { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC' }).format(new Date(this.week() + 'T12:00:00Z')));

  private initialView(): DemoView {
    const view = this.route.snapshot.queryParamMap.get('view');
    return view === 'team' || view === 'rosters' ? view : 'overview';
  }
  changeWeek(direction: number): void {
    const date = new Date(this.week() + 'T12:00:00Z');
    date.setUTCDate(date.getUTCDate() + direction * 7);
    this.week.set(date.toISOString().slice(0, 10));
    this.selectedShift.set(null);
  }
  requireAccount(action: string): void {
    this.action.set(action);
    this.dialog().nativeElement.showModal();
  }
  closePrompt(): void { this.dialog().nativeElement.close(); }
}
