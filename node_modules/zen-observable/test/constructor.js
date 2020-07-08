import assert from 'assert';
import { testMethodProperty } from './properties.js';

describe('constructor', () => {
  it('throws if called as a function', () => {
    assert.throws(() => Observable(() => {}));
    assert.throws(() => Observable.call({}, () => {}));
  });

  it('throws if the argument is not callable', () => {
    assert.throws(() => new Observable({}));
    assert.throws(() => new Observable());
    assert.throws(() => new Observable(1));
    assert.throws(() => new Observable('string'));
  });

  it('accepts a function argument', () => {
    let result = new Observable(() => {});
    assert.ok(result instanceof Observable);
  });

  it('is the value of Observable.prototype.constructor', () => {
    testMethodProperty(Observable.prototype, 'constructor', {
      configurable: true,
      writable: true,
      length: 1,
    });
  });

  it('does not call the subscriber function', () => {
    let called = 0;
    new Observable(() => { called++ });
    assert.equal(called, 0);
  });

});
