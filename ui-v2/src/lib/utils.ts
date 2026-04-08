import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

// KAI-307: shadcn/ui standard cn() helper.
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs));
}
