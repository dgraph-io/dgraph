import assert from 'assert';

describe('map', () => {
  it('maps the results using the supplied callback', async () => {
    let list = [];

    await Observable.from([1, 2, 3])
      .map(x => x * 2)
      .forEach(x => list.push(x));

    assert.deepEqual(list, [2, 4, 6]);
  });
});
