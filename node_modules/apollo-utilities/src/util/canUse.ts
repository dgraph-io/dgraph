export const canUseWeakMap = typeof WeakMap === 'function' && !(
  typeof navigator === 'object' &&
  navigator.product === 'ReactNative'
);
