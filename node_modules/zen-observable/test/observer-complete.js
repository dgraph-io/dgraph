import assert from 'assert';
import { testMethodProperty } from './properties.js';

describe('observer.complete', () => {

  function getObserver(inner) {
    let observer;
    new Observable(x => { observer = x }).subscribe(inner);
    return observer;
  }

  it('is a method of SubscriptionObserver', () => {
    let observer = getObserver();
    testMethodProperty(Object.getPrototypeOf(observer), 'complete', {
      configurable: true,
      writable: true,
      length: 0,
    });
  });

  it('does not forward arguments', () => {
    let args;
    let observer = getObserver({ complete(...a) { args = a } });
    observer.complete(1);
    assert.deepEqual(args, []);
  });

  it('does not return a value', () => {
    let observer = getObserver({ complete() { return 1 } });
    assert.equal(observer.complete(), undefined);
  });

  it('does not forward when the subscription is complete', () => {
    let count = 0;
    let observer = getObserver({ complete() { count++ } });
    observer.complete();
    observer.complete();
    assert.equal(count, 1);
  });

  it('does not forward when the subscription is cancelled', () => {
    let count = 0;
    let observer;
    let subscription = new Observable(x => { observer = x }).subscribe({
      complete() { count++ },
    });
    subscription.unsubscribe();
    observer.complete();
    assert.equal(count, 0);
  });

  it('queues if the subscription is not initialized', async () => {
    let completed = false;
    new Observable(x => { x.complete() }).subscribe({
      complete() { completed = true },
    });
    assert.equal(completed, false);
    await null;
    assert.equal(completed, true);
  });

  it('queues if the observer is running', async () => {
    let observer;
    let completed = false
    new Observable(x => { observer = x }).subscribe({
      next() { observer.complete() },
      complete() { completed = true },
    });
    observer.next();
    assert.equal(completed, false);
    await null;
    assert.equal(completed, true);
  });

  it('closes the subscription before invoking inner observer', () => {
    let closed;
    let observer = getObserver({
      complete() { closed = observer.closed },
    });
    observer.complete();
    assert.equal(closed, true);
  });

  it('reports error if "complete" is not a method', () => {
    let observer = getObserver({ complete: 1 });
    observer.complete();
    assert.ok(hostError instanceof Error);
  });

  it('does not report error if "complete" is undefined', () => {
    let observer = getObserver({ complete: undefined });
    observer.complete();
    assert.ok(!hostError);
  });

  it('does not report error if "complete" is null', () => {
    let observer = getObserver({ complete: null });
    observer.complete();
    assert.ok(!hostError);
  });

  it('reports error if "complete" throws', () => {
    let error = {};
    let observer = getObserver({ complete() { throw error } });
    observer.complete();
    assert.equal(hostError, error);
  });

  it('calls the cleanup method after "complete"', () => {
    let calls = [];
    let observer;
    new Observable(x => {
      observer = x;
      return () => { calls.push('cleanup') };
    }).subscribe({
      complete() { calls.push('complete') },
    });
    observer.complete();
    assert.deepEqual(calls, ['complete', 'cleanup']);
  });

  it('calls the cleanup method if there is no "complete"', () => {
    let calls = [];
    let observer;
    new Observable(x => {
      observer = x;
      return () => { calls.push('cleanup') };
    }).subscribe({});
    observer.complete();
    assert.deepEqual(calls, ['cleanup']);
  });

  it('reports error if the cleanup function throws', () => {
    let error = {};
    let observer;
    new Observable(x => {
      observer = x;
      return () => { throw error };
    }).subscribe();
    observer.complete();
    assert.equal(hostError, error);
  });
});
