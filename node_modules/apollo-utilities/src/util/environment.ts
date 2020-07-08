export function getEnv(): string | undefined {
  if (typeof process !== 'undefined' && process.env.NODE_ENV) {
    return process.env.NODE_ENV;
  }

  // default environment
  return 'development';
}

export function isEnv(env: string): boolean {
  return getEnv() === env;
}

export function isProduction(): boolean {
  return isEnv('production') === true;
}

export function isDevelopment(): boolean {
  return isEnv('development') === true;
}

export function isTest(): boolean {
  return isEnv('test') === true;
}
