import { Observable } from './Observable.js';

// Emits all values from all inputs in parallel
export function merge(...sources) {
  return new Observable(observer => {
    if (sources.length === 0)
      return Observable.from([]);

    let count = sources.length;

    let subscriptions = sources.map(source => Observable.from(source).subscribe({
      next(v) {
        observer.next(v);
      },
      error(e) {
        observer.error(e);
      },
      complete() {
        if (--count === 0)
          observer.complete();
      },
    }));

    return () => subscriptions.forEach(s => s.unsubscribe());
  });
}

// Emits arrays containing the most current values from each input
export function combineLatest(...sources) {
  return new Observable(observer => {
    if (sources.length === 0)
      return Observable.from([]);

    let count = sources.length;
    let seen = new Set();
    let seenAll = false;
    let values = sources.map(() => undefined);

    let subscriptions = sources.map((source, index) => Observable.from(source).subscribe({
      next(v) {
        values[index] = v;

        if (!seenAll) {
          seen.add(index);
          if (seen.size !== sources.length)
            return;

          seen = null;
          seenAll = true;
        }

        observer.next(Array.from(values));
      },
      error(e) {
        observer.error(e);
      },
      complete() {
        if (--count === 0)
          observer.complete();
      },
    }));

    return () => subscriptions.forEach(s => s.unsubscribe());
  });
}

// Emits arrays containing the matching index values from each input
export function zip(...sources) {
  return new Observable(observer => {
    if (sources.length === 0)
      return Observable.from([]);

    let queues = sources.map(() => []);

    function done() {
      return queues.some((q, i) => q.length === 0 && subscriptions[i].closed);
    }

    let subscriptions = sources.map((source, index) => Observable.from(source).subscribe({
      next(v) {
        queues[index].push(v);
        if (queues.every(q => q.length > 0)) {
          observer.next(queues.map(q => q.shift()));
          if (done())
            observer.complete();
        }
      },
      error(e) {
        observer.error(e);
      },
      complete() {
        if (done())
          observer.complete();
      },
    }));

    return () => subscriptions.forEach(s => s.unsubscribe());
  });
}
