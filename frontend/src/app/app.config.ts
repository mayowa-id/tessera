import { ApplicationConfig, provideBrowserGlobalErrorListeners } from '@angular/core';
import { provideRouter } from '@angular/router';
import { provideHttpClient, withInterceptors } from '@angular/common/http';
import { sessionInterceptor } from './auth.service';
import { provideApiConfiguration } from './api/api-configuration';
import { routes } from './app.routes';
export const appConfig: ApplicationConfig = { providers: [provideBrowserGlobalErrorListeners(), provideRouter(routes), provideApiConfiguration(''), provideHttpClient(withInterceptors([sessionInterceptor]))] };
