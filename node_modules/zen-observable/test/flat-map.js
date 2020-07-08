import assert from 'assert';

describe('flatMap', () => {
  it('maps and flattens the results using the supplied callback', async () => {
    let list = [];

    await Observable.of('a', 'b', 'c').flatMap(x =>
      Observable.of(1, 2, 3).map(y => [x, y])
    ).forEach(x => list.push(x));

    assert.deepEqual(list, [
      ['a', 1],
      ['a', 2],
      ['a', 3],
      ['b', 1],
      ['b', 2],
      ['b', 3],
      ['c', 1],
      ['c', 2],
      ['c', 3],
    ]);
  });
});
