import assert from 'assert';
import { testMethodProperty } from './properties.js';

describe('subscribe', () => {

  it('is a method of Observable.prototype', () => {
    testMethodProperty(Observable.prototype, 'subscribe', {
      configurable: true,
      writable: true,
      length: 1,
    });
  });

  it('accepts an observer argument', () => {
    let observer;
    let nextValue;
    new Observable(x => observer = x).subscribe({
      next(v) { nextValue = v },
    });
    observer.next(1);
    assert.equal(nextValue, 1);
  });

  it('accepts a next function argument', () => {
    let observer;
    let nextValue;
    new Observable(x => observer = x).subscribe(
      v => nextValue = v
    );
    observer.next(1);
    assert.equal(nextValue, 1);
  });

  it('accepts an error function argument', () => {
    let observer;
    let errorValue;
    let error = {};
    new Observable(x => observer = x).subscribe(
      null,
      e => errorValue = e
    );
    observer.error(error);
    assert.equal(errorValue, error);
  });

  it('accepts a complete function argument', () => {
    let observer;
    let completed = false;
    new Observable(x => observer = x).subscribe(
      null,
      null,
      () => completed = true
    );
    observer.complete();
    assert.equal(completed, true);
  });

  it('uses function overload if first argument is null', () => {
    let observer;
    let completed = false;
    new Observable(x => observer = x).subscribe(
      null,
      null,
      () => completed = true
    );
    observer.complete();
    assert.equal(completed, true);
  });

  it('uses function overload if first argument is undefined', () => {
    let observer;
    let completed = false;
    new Observable(x => observer = x).subscribe(
      undefined,
      null,
      () => completed = true
    );
    observer.complete();
    assert.equal(completed, true);
  });

  it('uses function overload if first argument is a primative', () => {
    let observer;
    let completed = false;
    new Observable(x => observer = x).subscribe(
      'abc',
      null,
      () => completed = true
    );
    observer.complete();
    assert.equal(completed, true);
  });

  it('enqueues a job to send error if subscriber throws', async () => {
    let error = {};
    let errorValue = undefined;
    new Observable(() => { throw error }).subscribe({
      error(e) { errorValue = e },
    });
    assert.equal(errorValue, undefined);
    await null;
    assert.equal(errorValue, error);
  });

  it('does not send error if unsubscribed', async () => {
    let error = {};
    let errorValue = undefined;
    let subscription = new Observable(() => { throw error }).subscribe({
      error(e) { errorValue = e },
    });
    subscription.unsubscribe();
    assert.equal(errorValue, undefined);
    await null;
    assert.equal(errorValue, undefined);
  });

  it('accepts a cleanup function from the subscriber function', () => {
    let cleanupCalled = false;
    let subscription = new Observable(() => {
      return () => cleanupCalled = true;
    }).subscribe();
    subscription.unsubscribe();
    assert.equal(cleanupCalled, true);
  });

  it('accepts a subscription object from the subscriber function', () => {
    let cleanupCalled = false;
    let subscription = new Observable(() => {
      return {
        unsubscribe() { cleanupCalled = true },
      };
    }).subscribe();
    subscription.unsubscribe();
    assert.equal(cleanupCalled, true);
  });

});
