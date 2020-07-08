import { isEnv, isProduction, isDevelopment, isTest } from '../environment';

describe('environment', () => {
  let keepEnv: string | undefined;

  beforeEach(() => {
    // save the NODE_ENV
    keepEnv = process.env.NODE_ENV;
  });

  afterEach(() => {
    // restore the NODE_ENV
    process.env.NODE_ENV = keepEnv;
  });

  describe('isEnv', () => {
    it(`should match when there's a value`, () => {
      ['production', 'development', 'test'].forEach(env => {
        process.env.NODE_ENV = env;
        expect(isEnv(env)).toBe(true);
      });
    });

    it(`should treat no proces.env.NODE_ENV as it'd be in development`, () => {
      delete process.env.NODE_ENV;
      expect(isEnv('development')).toBe(true);
    });
  });

  describe('isProduction', () => {
    it('should return true if in production', () => {
      process.env.NODE_ENV = 'production';
      expect(isProduction()).toBe(true);
    });

    it('should return false if not in production', () => {
      process.env.NODE_ENV = 'test';
      expect(!isProduction()).toBe(true);
    });
  });

  describe('isTest', () => {
    it('should return true if in test', () => {
      process.env.NODE_ENV = 'test';
      expect(isTest()).toBe(true);
    });

    it('should return true if not in test', () => {
      process.env.NODE_ENV = 'development';
      expect(!isTest()).toBe(true);
    });
  });

  describe('isDevelopment', () => {
    it('should return true if in development', () => {
      process.env.NODE_ENV = 'development';
      expect(isDevelopment()).toBe(true);
    });

    it('should return true if not in development and environment is defined', () => {
      process.env.NODE_ENV = 'test';
      expect(!isDevelopment()).toBe(true);
    });

    it('should make development as the default environment', () => {
      delete process.env.NODE_ENV;
      expect(isDevelopment()).toBe(true);
    });
  });
});
