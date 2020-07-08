import { warnOnceInDevelopment } from '../warnOnce';

let lastWarning: string | null;
let keepEnv: string | undefined;
let numCalls = 0;
let oldConsoleWarn: any;

describe('warnOnce', () => {
  beforeEach(() => {
    keepEnv = process.env.NODE_ENV;
    numCalls = 0;
    lastWarning = null;
    oldConsoleWarn = console.warn;
    console.warn = (msg: any) => {
      numCalls++;
      lastWarning = msg;
    };
  });
  afterEach(() => {
    process.env.NODE_ENV = keepEnv;
    console.warn = oldConsoleWarn;
  });
  it('actually warns', () => {
    process.env.NODE_ENV = 'development';
    warnOnceInDevelopment('hi');
    expect(lastWarning).toBe('hi');
    expect(numCalls).toEqual(1);
  });

  it('does not warn twice', () => {
    process.env.NODE_ENV = 'development';
    warnOnceInDevelopment('ho');
    warnOnceInDevelopment('ho');
    expect(lastWarning).toEqual('ho');
    expect(numCalls).toEqual(1);
  });

  it('warns two different things once each', () => {
    process.env.NODE_ENV = 'development';
    warnOnceInDevelopment('slow');
    expect(lastWarning).toEqual('slow');
    warnOnceInDevelopment('mo');
    expect(lastWarning).toEqual('mo');
    expect(numCalls).toEqual(2);
  });

  it('does not warn in production', () => {
    process.env.NODE_ENV = 'production';
    warnOnceInDevelopment('lo');
    warnOnceInDevelopment('lo');
    expect(numCalls).toEqual(0);
  });

  it('warns many times in test', () => {
    process.env.NODE_ENV = 'test';
    warnOnceInDevelopment('yo');
    warnOnceInDevelopment('yo');
    expect(lastWarning).toEqual('yo');
    expect(numCalls).toEqual(2);
  });
});
