import { Component, computed, input } from '@angular/core';
import { LucideDynamicIcon, LucideSun, LucideMoon, LucideMonitor, LucideArrowUpRight, LucideArrowDown, LucideArrowLeft, LucideArrowRight } from '@lucide/angular';

const icons = {
  sun: LucideSun, moon: LucideMoon, monitor: LucideMonitor,
  'arrow-up-right': LucideArrowUpRight, 'arrow-down': LucideArrowDown,
  'arrow-left': LucideArrowLeft, 'arrow-right': LucideArrowRight,
};

// One size, stroke and color treatment for every decorative icon in the app.
@Component({
  selector: 'app-icon', imports: [LucideDynamicIcon],
  host: { 'aria-hidden': 'true' },
  template: '<svg [lucideIcon]="icon()" [size]="18" [strokeWidth]="1.75" aria-hidden="true" focusable="false"></svg>',
  styles: [':host{display:inline-flex;align-items:center;justify-content:center;vertical-align:middle;line-height:0;flex-shrink:0}'],
})
export class Icon {
  readonly name = input.required<keyof typeof icons>();
  readonly icon = computed(() => icons[this.name()]);
}
