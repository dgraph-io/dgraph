import assert from 'assert';

describe('forEach', () => {

  it('rejects if the argument is not a function', async () => {
    let promise = Observable.of(1, 2, 3).forEach();
    try {
      await promise;
      assert.ok(false);
    } catch (err) {
      assert.equal(err.name, 'TypeError');
    }
  });

  it('rejects if the callback throws', async () => {
    let error = {};
    try {
      await Observable.of(1, 2, 3).forEach(x => { throw error });
      assert.ok(false);
    } catch (err) {
      assert.equal(err, error);
    }
  });

  it('does not execute callback after callback throws', async () => {
    let calls = [];
    try {
      await Observable.of(1, 2, 3).forEach(x => {
        calls.push(x);
        throw {};
      });
      assert.ok(false);
    } catch (err) {
      assert.deepEqual(calls, [1]);
    }
  });

  it('rejects if the producer calls error', async () => {
    let error = {};
    try {
      let observer;
      let promise = new Observable(x => { observer = x }).forEach(() => {});
      observer.error(error);
      await promise;
      assert.ok(false);
    } catch (err) {
      assert.equal(err, error);
    }
  });

  it('resolves with undefined if the producer calls complete', async () => {
    let observer;
    let promise = new Observable(x => { observer = x }).forEach(() => {});
    observer.complete();
    assert.equal(await promise, undefined);
  });

  it('provides a cancellation function as the second argument', async () => {
    let observer;
    let results = [];
    await Observable.of(1, 2, 3).forEach((value, cancel) => {
      results.push(value);
      if (value > 1) {
        return cancel();
      }
    });
    assert.deepEqual(results, [1, 2]);
  });

});
