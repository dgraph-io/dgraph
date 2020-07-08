import assert from 'assert';
import { testMethodProperty } from './properties.js';

describe('observer.error', () => {

  function getObserver(inner) {
    let observer;
    new Observable(x => { observer = x }).subscribe(inner);
    return observer;
  }

  it('is a method of SubscriptionObserver', () => {
    let observer = getObserver();
    testMethodProperty(Object.getPrototypeOf(observer), 'error', {
      configurable: true,
      writable: true,
      length: 1,
    });
  });

  it('forwards the argument', () => {
    let args;
    let observer = getObserver({ error(...a) { args = a } });
    observer.error(1);
    assert.deepEqual(args, [1]);
  });

  it('does not return a value', () => {
    let observer = getObserver({ error() { return 1 } });
    assert.equal(observer.error(), undefined);
  });

  it('does not throw when the subscription is complete', () => {
    let observer = getObserver({ error() {} });
    observer.complete();
    observer.error('error');
  });

  it('does not throw when the subscription is cancelled', () => {
    let observer;
    let subscription = new Observable(x => { observer = x }).subscribe({
      error() {},
    });
    subscription.unsubscribe();
    observer.error(1);
    assert.ok(!hostError);
  });

  it('queues if the subscription is not initialized', async () => {
    let error;
    new Observable(x => { x.error({}) }).subscribe({
      error(err) { error = err },
    });
    assert.equal(error, undefined);
    await null;
    assert.ok(error);
  });

  it('queues if the observer is running', async () => {
    let observer;
    let error;
    new Observable(x => { observer = x }).subscribe({
      next() { observer.error({}) },
      error(e) { error = e },
    });
    observer.next();
    assert.ok(!error);
    await null;
    assert.ok(error);
  });

  it('closes the subscription before invoking inner observer', () => {
    let closed;
    let observer = getObserver({
      error() { closed = observer.closed },
    });
    observer.error(1);
    assert.equal(closed, true);
  });

  it('reports an error if "error" is not a method', () => {
    let observer = getObserver({ error: 1 });
    observer.error(1);
    assert.ok(hostError);
  });

  it('reports an error if "error" is undefined', () => {
    let error = {};
    let observer = getObserver({ error: undefined });
    observer.error(error);
    assert.equal(hostError, error);
  });

  it('reports an error if "error" is null', () => {
    let error = {};
    let observer = getObserver({ error: null });
    observer.error(error);
    assert.equal(hostError, error);
  });

  it('reports error if "error" throws', () => {
    let error = {};
    let observer = getObserver({ error() { throw error } });
    observer.error(1);
    assert.equal(hostError, error);
  });

  it('calls the cleanup method after "error"', () => {
    let calls = [];
    let observer;
    new Observable(x => {
      observer = x;
      return () => { calls.push('cleanup') };
    }).subscribe({
      error() { calls.push('error') },
    });
    observer.error();
    assert.deepEqual(calls, ['error', 'cleanup']);
  });

  it('calls the cleanup method if there is no "error"', () => {
    let calls = [];
    let observer;
    new Observable(x => {
      observer = x;
      return () => { calls.push('cleanup') };
    }).subscribe({});
    try {
      observer.error();
    } catch (err) {}
    assert.deepEqual(calls, ['cleanup']);
  });

  it('reports error if the cleanup function throws', () => {
    let error = {};
    let observer;
    new Observable(x => {
      observer = x;
      return () => { throw error };
    }).subscribe();
    observer.error(1);
    assert.equal(hostError, error);
  });

});
