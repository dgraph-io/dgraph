import assert from 'assert';
import { testMethodProperty } from './properties.js';

describe('from', () => {
  const iterable = {
    *[Symbol.iterator]() {
      yield 1;
      yield 2;
      yield 3;
    },
  };

  it('is a method on Observable', () => {
    testMethodProperty(Observable, 'from', {
      configurable: true,
      writable: true,
      length: 1,
    });
  });

  it('throws if the argument is null', () => {
    assert.throws(() => Observable.from(null));
  });

  it('throws if the argument is undefined', () => {
    assert.throws(() => Observable.from(undefined));
  });

  it('throws if the argument is not observable or iterable', () => {
    assert.throws(() => Observable.from({}));
  });

  describe('observables', () => {
    it('returns the input if the constructor matches "this"', () => {
      let ctor = function() {};
      let observable = new Observable(() => {});
      observable.constructor = ctor;
      assert.equal(Observable.from.call(ctor, observable), observable);
    });

    it('wraps the input if it is not an instance of Observable', () => {
      let obj = {
        'constructor': Observable,
        [Symbol.observable]() { return this },
      };
      assert.ok(Observable.from(obj) !== obj);
    });

    it('throws if @@observable property is not a method', () => {
      assert.throws(() => Observable.from({
        [Symbol.observable]: 1
      }));
    });

    it('returns an observable wrapping @@observable result', () => {
      let inner = {
        subscribe(x) {
          observer = x;
          return () => { cleanupCalled = true };
        },
      };
      let observer;
      let cleanupCalled = true;
      let observable = Observable.from({
        [Symbol.observable]() { return inner },
      });
      observable.subscribe();
      assert.equal(typeof observer.next, 'function');
      observer.complete();
      assert.equal(cleanupCalled, true);
    });
  });

  describe('iterables', () => {
    it('throws if @@iterator is not a method', () => {
      assert.throws(() => Observable.from({ [Symbol.iterator]: 1 }));
    });

    it('returns an observable wrapping iterables', async () => {
      let calls = [];
      let subscription = Observable.from(iterable).subscribe({
        next(v) { calls.push(['next', v]) },
        complete() { calls.push(['complete']) },
      });
      assert.deepEqual(calls, []);
      await null;
      assert.deepEqual(calls, [
        ['next', 1],
        ['next', 2],
        ['next', 3],
        ['complete'],
      ]);
    });
  });
});
