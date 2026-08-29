// streams.go implements amifl-spec.md sections 11/13.8's `parallel(s,
// workers) -> Stream[T]` — step 12. Every other Stream[T] operation
// (`take`/`skip`/`collect`, and `lines`) is hand-rolled directly as AMIVM-IR
// by codegen (chan.go/files.go — CHRECV/CHSEND/CLOS/SPAWN/DEFER are enough
// on their own); `parallel` is the one exception, needing a real completion
// count across N worker goroutines before it can close its output channel,
// which AMIVM has no synchronization instruction for at all (no atomic/
// mutex instruction exists) — exactly the "専用のGoランタイムパッケージを
// 新設しないと実現不能" criterion CLAUDE.md's design notes call for.
package amiflrt

import "sync"

// ParallelStream implements `parallel(s: Stream[T], workers: Int) ->
// Stream[T]`: distributes reads from in across `workers` goroutines, each
// relaying everything it receives into a freshly-made out channel, closed
// once every worker has drained in to completion (amifl-spec.md section 11:
// "内部でN個のgoroutineへ分配する").
func ParallelStream[T any](in chan T, workers int) chan T {
	out := make(chan T)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for v := range in {
				out <- v
			}
		}()
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}
