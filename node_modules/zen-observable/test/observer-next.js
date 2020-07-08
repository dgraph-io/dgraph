import assert from 'assert';
import { testMethodProperty } from './properties.js';

describe('observer.next', () => {

  function getObserver(inner) {
    let observer;
    new Observable(x => { observer = x }).subscribe(inner);
    return observer;
  }

  it('is a method of SubscriptionObserver', () => {
    let observer = getObserver();
    testMethodProperty(Object.getPrototypeOf(observer), 'next', {
      configurable: true,
      writable: true,
      length: 1,
    });
  });

  it('forwards the first argument', () => {
    let args;
    let observer = getObserver({ next(...a) { args = a } });
    observer.next(1, 2);
    assert.deepEqual(args, [1]);
  });

  it('does not return a value', () => {
    let observer = getObserver({ next() { return 1 } });
    assert.equal(observer.next(), undefined);
  });

  it('does not forward when the subscription is complete', () => {
    let count = 0;
    let observer = getObserver({ next() { count++ } });
    observer.complete();
    observer.next();
    assert.equal(count, 0);
  });

  it('does not forward when the subscription is cancelled', () => {
    let count = 0;
    let observer;
    let subscription = new Observable(x => { observer = x }).subscribe({
      next() { count++ },
    });
    subscription.unsubscribe();
    observer.next();
    assert.equal(count, 0);
  });

  it('remains closed if the subscription is cancelled from "next"', () => {
    let observer;
    let subscription = new Observable(x => { observer = x }).subscribe({
      next() { subscription.unsubscribe() },
    });
    observer.next();
    assert.equal(observer.closed, true);
  });

  it('queues if the subscription is not initialized', async () => {
    let values = [];
    let observer;
    new Observable(x => { observer = x, x.next(1) }).subscribe({
      next(val) {
        values.push(val);
        if (val === 1) {
          observer.next(3);
        }
      },
    });
    observer.next(2);
    assert.deepEqual(values, []);
    await null;
    assert.deepEqual(values, [1, 2]);
    await null;
    assert.deepEqual(values, [1, 2, 3]);
  });

  it('drops queue if subscription is closed', async () => {
    let values = [];
    let subscription = new Observable(x => { x.next(1) }).subscribe({
      next(val) { values.push(val) },
    });
    assert.deepEqual(values, []);
    subscription.unsubscribe();
    await null;
    assert.deepEqual(values, []);
  });

  it('queues if the observer is running', async () => {
    let observer;
    let values = [];
    new Observable(x => { observer = x }).subscribe({
      next(val) {
        values.push(val);
        if (val === 1) observer.next(2);
      },
    });
    observer.next(1);
    assert.deepEqual(values, [1]);
    await null;
    assert.deepEqual(values, [1, 2]);
  });

  it('reports error if "next" is not a method', () => {
    let observer = getObserver({ next: 1 });
    observer.next();
    assert.ok(hostError);
  });

  it('does not report error if "next" is undefined', () => {
    let observer = getObserver({ next: undefined });
    observer.next();
    assert.ok(!hostError);
  });

  it('does not report error if "next" is null', () => {
    let observer = getObserver({ next: null });
    observer.next();
    assert.ok(!hostError);
  });

  it('reports error if "next" throws', () => {
    let error = {};
    let observer = getObserver({ next() { throw error } });
    observer.next();
    assert.equal(hostError, error);
  });

  it('does not close the subscription on error', () => {
    let observer = getObserver({ next() { throw {} } });
    observer.next();
    assert.equal(observer.closed, false);
  });

});
