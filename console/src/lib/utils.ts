import { type ClassValue, clsx } from 'clsx';
import { twMerge } from 'tailwind-merge';

/** Joins class names, with Tailwind's own merge resolving two utilities that set the same
 *  property. */
export function cn(...inputs: ClassValue[]) {
	return twMerge(clsx(inputs));
}
