// @flow strict

export type ObjMap<T> = { [key: string]: T, __proto__: null, ... };
export type ObjMapLike<T> = ObjMap<T> | { [key: string]: T, ... };

export type ReadOnlyObjMap<T> = { +[key: string]: T, __proto__: null, ... };
export type ReadOnlyObjMapLike<T> =
  | ReadOnlyObjMap<T>
  | { +[key: string]: T, ... };
