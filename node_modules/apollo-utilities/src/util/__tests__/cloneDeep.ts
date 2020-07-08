import { cloneDeep } from '../cloneDeep';

describe('cloneDeep', () => {
  it('will clone primitive values', () => {
    expect(cloneDeep(undefined)).toEqual(undefined);
    expect(cloneDeep(null)).toEqual(null);
    expect(cloneDeep(true)).toEqual(true);
    expect(cloneDeep(false)).toEqual(false);
    expect(cloneDeep(-1)).toEqual(-1);
    expect(cloneDeep(+1)).toEqual(+1);
    expect(cloneDeep(0.5)).toEqual(0.5);
    expect(cloneDeep('hello')).toEqual('hello');
    expect(cloneDeep('world')).toEqual('world');
  });

  it('will clone objects', () => {
    const value1 = {};
    const value2 = { a: 1, b: 2, c: 3 };
    const value3 = { x: { a: 1, b: 2, c: 3 }, y: { a: 1, b: 2, c: 3 } };

    const clonedValue1 = cloneDeep(value1);
    const clonedValue2 = cloneDeep(value2);
    const clonedValue3 = cloneDeep(value3);

    expect(clonedValue1).toEqual(value1);
    expect(clonedValue2).toEqual(value2);
    expect(clonedValue3).toEqual(value3);

    expect(clonedValue1).toEqual(value1);
    expect(clonedValue2).toEqual(value2);
    expect(clonedValue3).toEqual(value3);
    expect(clonedValue3.x).toEqual(value3.x);
    expect(clonedValue3.y).toEqual(value3.y);
  });

  it('will clone arrays', () => {
    const value1: Array<number> = [];
    const value2 = [1, 2, 3];
    const value3 = [[1, 2, 3], [1, 2, 3]];

    const clonedValue1 = cloneDeep(value1);
    const clonedValue2 = cloneDeep(value2);
    const clonedValue3 = cloneDeep(value3);

    expect(clonedValue1).toEqual(value1);
    expect(clonedValue2).toEqual(value2);
    expect(clonedValue3).toEqual(value3);

    expect(clonedValue1).toEqual(value1);
    expect(clonedValue2).toEqual(value2);
    expect(clonedValue3).toEqual(value3);
    expect(clonedValue3[0]).toEqual(value3[0]);
    expect(clonedValue3[1]).toEqual(value3[1]);
  });

  it('should not attempt to follow circular references', () => {
    const someObject = {
      prop1: 'value1',
      anotherObject: null,
    };
    const anotherObject = {
      someObject,
    };
    someObject.anotherObject = anotherObject;
    let chk;
    expect(() => {
      chk = cloneDeep(someObject);
    }).not.toThrow();
  });
});
