import { assign } from '../assign';

describe('assign', () => {
  it('will merge many objects together', () => {
    expect(assign({ a: 1 }, { b: 2 })).toEqual({ a: 1, b: 2 });
    expect(assign({ a: 1 }, { b: 2 }, { c: 3 })).toEqual({
      a: 1,
      b: 2,
      c: 3,
    });
    expect(assign({ a: 1 }, { b: 2 }, { c: 3 }, { d: 4 })).toEqual({
      a: 1,
      b: 2,
      c: 3,
      d: 4,
    });
  });

  it('will merge many objects together shallowly', () => {
    expect(assign({ x: { a: 1 } }, { x: { b: 2 } })).toEqual({ x: { b: 2 } });
    expect(assign({ x: { a: 1 } }, { x: { b: 2 } }, { x: { c: 3 } })).toEqual({
      x: { c: 3 },
    });
    expect(
      assign(
        { x: { a: 1 } },
        { x: { b: 2 } },
        { x: { c: 3 } },
        { x: { d: 4 } },
      ),
    ).toEqual({ x: { d: 4 } });
  });

  it('will mutate and return the source objects', () => {
    const source1 = { a: 1 };
    const source2 = { a: 1 };
    const source3 = { a: 1 };

    expect(assign(source1, { b: 2 })).toEqual(source1);
    expect(assign(source2, { b: 2 }, { c: 3 })).toEqual(source2);
    expect(assign(source3, { b: 2 }, { c: 3 }, { d: 4 })).toEqual(source3);

    expect(source1).toEqual({ a: 1, b: 2 });
    expect(source2).toEqual({ a: 1, b: 2, c: 3 });
    expect(source3).toEqual({ a: 1, b: 2, c: 3, d: 4 });
  });
});
