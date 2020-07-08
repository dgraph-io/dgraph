import assert from 'assert';

describe('reduce', () => {
  it('reduces without a seed', async () => {
    await Observable.from([1, 2, 3, 4, 5, 6]).reduce((a, b) => {
      return a + b;
    }).forEach(x => {
      assert.equal(x, 21);
    });
  });

  it('errors if empty and no seed', async () => {
    try {
      await Observable.from([]).reduce((a, b) => {
        return a + b;
      }).forEach(() => null);
      assert.ok(false);
    } catch (err) {
      assert.ok(true);
    }
  });

  it('reduces with a seed', async () => {
    Observable.from([1, 2, 3, 4, 5, 6]).reduce((a, b) => {
      return a + b;
    }, 100).forEach(x => {
      assert.equal(x, 121);
    });
  });

  it('reduces an empty list with a seed', async () => {
    await Observable.from([]).reduce((a, b) => {
      return a + b;
    }, 100).forEach(x => {
      assert.equal(x, 100);
    });
  });
});
