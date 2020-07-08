import assert from 'assert';
import { testMethodProperty } from './properties.js';

describe('observer.closed', () => {
  it('is a getter on SubscriptionObserver.prototype', () => {
    let observer;
    new Observable(x => { observer = x }).subscribe();
    testMethodProperty(Object.getPrototypeOf(observer), 'closed', {
      get: true,
      configurable: true,
      writable: true,
      length: 1
    });
  });

  it('returns false when the subscription is open', () => {
    new Observable(observer => {
      assert.equal(observer.closed, false);
    }).subscribe();
  });

  it('returns true when the subscription is completed', () => {
    let observer;
    new Observable(x => { observer = x; }).subscribe();
    observer.complete();
    assert.equal(observer.closed, true);
  });

  it('returns true when the subscription is errored', () => {
    let observer;
    new Observable(x => { observer = x; }).subscribe(null, () => {});
    observer.error();
    assert.equal(observer.closed, true);
  });
});
