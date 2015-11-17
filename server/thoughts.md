1. Select v/s Range

2. sync.WaitGroup.

func handle(..) {
	wg.Add(1)
	...
	wg.Done()
}

func main() {
	wg := new(sync.WaitGroup)
	for i := 0; i < N; i++ {
		go handle(..)
	}
	wg.Wait()
}

The above wouldn't work, because goroutines don't necessarily get scheduled immediately.
So, wg.Add(1) wouldn't get called, which means wg.Wait() wouldn't block, and the program
would finish execution before goroutines had a chance to be run.
