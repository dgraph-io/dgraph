import assert from 'assert';

describe('filter', () => {
  it('filters the results using the supplied callback', async () => {
    let list = [];

    await Observable
      .from([1, 2, 3, 4])
      .filter(x => x > 2)
      .forEach(x => list.push(x));

    assert.deepEqual(list, [3, 4]);
  });
});
