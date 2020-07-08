import { isEqual } from '../isEqual';

describe('isEqual', () => {
  it('should return true for equal primitive values', () => {
    expect(isEqual(undefined, undefined)).toBe(true);
    expect(isEqual(null, null)).toBe(true);
    expect(isEqual(true, true)).toBe(true);
    expect(isEqual(false, false)).toBe(true);
    expect(isEqual(-1, -1)).toBe(true);
    expect(isEqual(+1, +1)).toBe(true);
    expect(isEqual(42, 42)).toBe(true);
    expect(isEqual(0, 0)).toBe(true);
    expect(isEqual(0.5, 0.5)).toBe(true);
    expect(isEqual('hello', 'hello')).toBe(true);
    expect(isEqual('world', 'world')).toBe(true);
  });

  it('should return false for not equal primitive values', () => {
    expect(!isEqual(undefined, null)).toBe(true);
    expect(!isEqual(null, undefined)).toBe(true);
    expect(!isEqual(true, false)).toBe(true);
    expect(!isEqual(false, true)).toBe(true);
    expect(!isEqual(-1, +1)).toBe(true);
    expect(!isEqual(+1, -1)).toBe(true);
    expect(!isEqual(42, 42.00000000000001)).toBe(true);
    expect(!isEqual(0, 0.5)).toBe(true);
    expect(!isEqual('hello', 'world')).toBe(true);
    expect(!isEqual('world', 'hello')).toBe(true);
  });

  it('should return false when comparing primitives with objects', () => {
    expect(!isEqual({}, null)).toBe(true);
    expect(!isEqual(null, {})).toBe(true);
    expect(!isEqual({}, true)).toBe(true);
    expect(!isEqual(true, {})).toBe(true);
    expect(!isEqual({}, 42)).toBe(true);
    expect(!isEqual(42, {})).toBe(true);
    expect(!isEqual({}, 'hello')).toBe(true);
    expect(!isEqual('hello', {})).toBe(true);
  });

  it('should correctly compare shallow objects', () => {
    expect(isEqual({}, {})).toBe(true);
    expect(isEqual({ a: 1, b: 2, c: 3 }, { a: 1, b: 2, c: 3 })).toBe(true);
    expect(!isEqual({ a: 1, b: 2, c: 3 }, { a: 3, b: 2, c: 1 })).toBe(true);
    expect(!isEqual({ a: 1, b: 2, c: 3 }, { a: 1, b: 2 })).toBe(true);
    expect(!isEqual({ a: 1, b: 2 }, { a: 1, b: 2, c: 3 })).toBe(true);
  });

  it('should correctly compare deep objects', () => {
    expect(isEqual({ x: {} }, { x: {} })).toBe(true);
    expect(
      isEqual({ x: { a: 1, b: 2, c: 3 } }, { x: { a: 1, b: 2, c: 3 } }),
    ).toBe(true);
    expect(
      !isEqual({ x: { a: 1, b: 2, c: 3 } }, { x: { a: 3, b: 2, c: 1 } }),
    ).toBe(true);
    expect(!isEqual({ x: { a: 1, b: 2, c: 3 } }, { x: { a: 1, b: 2 } })).toBe(
      true,
    );
    expect(!isEqual({ x: { a: 1, b: 2 } }, { x: { a: 1, b: 2, c: 3 } })).toBe(
      true,
    );
  });

  it('should correctly compare deep objects without object prototype ', () => {
    // Solves https://github.com/apollographql/apollo-client/issues/2132
    const objNoProto = Object.create(null);
    objNoProto.a = { b: 2, c: [3, 4] };
    objNoProto.e = Object.create(null);
    objNoProto.e.f = 5;
    expect(isEqual(objNoProto, { a: { b: 2, c: [3, 4] }, e: { f: 5 } })).toBe(
      true,
    );
    expect(!isEqual(objNoProto, { a: { b: 2, c: [3, 4] }, e: { f: 6 } })).toBe(
      true,
    );
    expect(!isEqual(objNoProto, { a: { b: 2, c: [3, 4] }, e: null })).toBe(
      true,
    );
    expect(!isEqual(objNoProto, { a: { b: 2, c: [3] }, e: { f: 5 } })).toBe(
      true,
    );
    expect(!isEqual(objNoProto, null)).toBe(true);
  });

  it('should correctly handle modified prototypes', () => {
    Array.prototype.foo = null;
    expect(isEqual([1, 2, 3], [1, 2, 3])).toBe(true);
    expect(!isEqual([1, 2, 3], [1, 2, 4])).toBe(true);
    delete Array.prototype.foo;
  });

  describe('comparing objects with circular refs', () => {
    // copied with slight modification from lodash test suite
    it('should compare objects with circular references', () => {
      const object1 = {},
        object2 = {};

      object1.a = object1;
      object2.a = object2;

      expect(isEqual(object1, object2)).toBe(true);

      object1.b = 0;
      object2.b = Object(0);

      expect(isEqual(object1, object2)).toBe(true);

      object1.c = Object(1);
      object2.c = Object(2);

      expect(isEqual(object1, object2)).toBe(false);

      object1 = { a: 1, b: 2, c: 3 };
      object1.b = object1;
      object2 = { a: 1, b: { a: 1, b: 2, c: 3 }, c: 3 };

      expect(isEqual(object1, object2)).toBe(false);
    });

    it('should have transitive equivalence for circular references of objects', () => {
      const object1 = {},
        object2 = { a: object1 },
        object3 = { a: object2 };

      object1.a = object1;

      expect(isEqual(object1, object2)).toBe(true);
      expect(isEqual(object2, object3)).toBe(true);
      expect(isEqual(object1, object3)).toBe(true);
    });

    it('should compare objects with multiple circular references', () => {
      const array1 = [{}],
        array2 = [{}];

      (array1[0].a = array1).push(array1);
      (array2[0].a = array2).push(array2);

      expect(isEqual(array1, array2)).toBe(true);

      array1[0].b = 0;
      array2[0].b = Object(0);

      expect(isEqual(array1, array2)).toBe(true);

      array1[0].c = Object(1);
      array2[0].c = Object(2);

      expect(isEqual(array1, array2)).toBe(false);
    });

    it('should compare objects with complex circular references', () => {
      const object1 = {
        foo: { b: { c: { d: {} } } },
        bar: { a: 2 },
      };

      const object2 = {
        foo: { b: { c: { d: {} } } },
        bar: { a: 2 },
      };

      object1.foo.b.c.d = object1;
      object1.bar.b = object1.foo.b;

      object2.foo.b.c.d = object2;
      object2.bar.b = object2.foo.b;

      expect(isEqual(object1, object2)).toBe(true);
    });
  });
});
