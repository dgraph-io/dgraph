import assert from 'assert';
import { testMethodProperty } from './properties.js';

describe('of', () => {
  it('is a method on Observable', () => {
    testMethodProperty(Observable, 'of', {
      configurable: true,
      writable: true,
      length: 0,
    });
  });

  it('uses the this value if it is a function', () => {
    let usesThis = false;
    Observable.of.call(function() { usesThis = true; });
    assert.ok(usesThis);
  });

  it('uses Observable if the this value is not a function', () => {
    let result = Observable.of.call({}, 1, 2, 3, 4);
    assert.ok(result instanceof Observable);
  });

  it('delivers arguments to next in a job', async () => {
    let values = [];
    let turns = 0;
    Observable.of(1, 2, 3, 4).subscribe(v => values.push(v));
    assert.equal(values.length, 0);
    await null;
    assert.deepEqual(values, [1, 2, 3, 4]);
  });
});
