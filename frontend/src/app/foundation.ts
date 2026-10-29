import { Component, inject, signal } from '@angular/core';
import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import { timeout } from 'rxjs';

@Component({ selector: 'app-foundation', templateUrl: './foundation.html', styleUrl: './foundation.css' })
export class Foundation {
  private readonly http = inject(HttpClient);
  readonly state = signal<'checking' | 'ready' | 'unavailable'>('checking');
  readonly message = signal('');
  constructor() { this.checkConnection(); }
  checkConnection(): void {
    this.state.set('checking');
    this.message.set('Connecting to the Go API and PostgreSQL…');
    this.http.get<{ status: string }>('/api/readyz').pipe(timeout(5000)).subscribe({
      next: (response) => {
        const ready = response.status === 'ready';
        this.state.set(ready ? 'ready' : 'unavailable');
        this.message.set(ready ? 'The Go API is responding and PostgreSQL is connected.' : 'The API returned an unexpected readiness response.');
      },
      error: (error: unknown) => {
        this.state.set('unavailable');
        this.message.set(error instanceof HttpErrorResponse && error.status === 503
          ? 'The API is running, but PostgreSQL is unavailable. Start the database and try again.'
          : 'Unable to reach the API. Start the Go server and check the API proxy configuration.');
      },
    });
  }
}
