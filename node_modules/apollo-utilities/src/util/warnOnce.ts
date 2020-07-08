import { isProduction, isTest } from './environment';

const haveWarned = Object.create({});

/**
 * Print a warning only once in development.
 * In production no warnings are printed.
 * In test all warnings are printed.
 *
 * @param msg The warning message
 * @param type warn or error (will call console.warn or console.error)
 */
export function warnOnceInDevelopment(msg: string, type = 'warn') {
  if (!isProduction() && !haveWarned[msg]) {
    if (!isTest()) {
      haveWarned[msg] = true;
    }
    if (type === 'error') {
      console.error(msg);
    } else {
      console.warn(msg);
    }
  }
}
