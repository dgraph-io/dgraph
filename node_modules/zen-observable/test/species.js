import assert from 'assert';

describe('species', () => {
  it('uses Observable when constructor is undefined', () => {
    let instance = new Observable(() => {});
    instance.constructor = undefined;
    assert.ok(instance.map(x => x) instanceof Observable);
  });

  it('uses Observable if species is null', () => {
    let instance = new Observable(() => {});
    instance.constructor = { [Symbol.species]: null };
    assert.ok(instance.map(x => x) instanceof Observable);
  });

  it('uses Observable if species is undefined', () => {
    let instance = new Observable(() => {});
    instance.constructor = { [Symbol.species]: undefined };
    assert.ok(instance.map(x => x) instanceof Observable);
  });

  it('uses value of Symbol.species', () => {
    function ctor() {}
    let instance = new Observable(() => {});
    instance.constructor = { [Symbol.species]: ctor };
    assert.ok(instance.map(x => x) instanceof ctor);
  });
});
