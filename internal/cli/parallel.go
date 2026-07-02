package cli

import (
	"context"
	"runtime"
	"sync"
)

func readOnlyWorkerCount(repoCount int) int {
	return cappedWorkerCount(repoCount, 32)
}

func gitMutationWorkerCount(repoCount int) int {
	return cappedWorkerCount(repoCount, 4)
}

func cappedWorkerCount(repoCount, cap int) int {
	workers := runtime.NumCPU()
	if workers > cap {
		workers = cap
	}
	if workers < 1 {
		workers = 1
	}
	if repoCount > 0 && workers > repoCount {
		workers = repoCount
	}
	return workers
}

func parallelRepos[T any](ctx context.Context, repos []repo, inspect func(repo) T) []T {
	return parallelReposWithWorkers(ctx, repos, readOnlyWorkerCount(len(repos)), inspect)
}

func parallelReposProgress[T any](ctx context.Context, repos []repo, progress *progress, inspect func(repo) T) []T {
	return parallelReposWithWorkersProgress(ctx, repos, readOnlyWorkerCount(len(repos)), progress, inspect)
}

func parallelGitMutationsProgress[T any](ctx context.Context, repos []repo, progress *progress, mutate func(repo) T) []T {
	return parallelReposWithWorkersProgress(ctx, repos, gitMutationWorkerCount(len(repos)), progress, mutate)
}

func parallelReposWithWorkers[T any](ctx context.Context, repos []repo, workers int, inspect func(repo) T) []T {
	return parallelReposWithWorkersProgress(ctx, repos, workers, nil, inspect)
}

func parallelReposWithWorkersProgress[T any](ctx context.Context, repos []repo, workers int, progress *progress, inspect func(repo) T) []T {
	return parallelItemsWithWorkersProgress(ctx, repos, workers, progress, func(r repo) (string, string) {
		return r.display, r.display
	}, inspect)
}

func parallelItemsWithWorkersProgress[T any, R any](ctx context.Context, items []T, workers int, progress *progress, detail func(T) (string, string), work func(T) R) []R {
	if ctx == nil {
		ctx = context.Background()
	}
	results := make([]R, len(items))
	jobs := make(chan int)
	var wg sync.WaitGroup
	defer progress.done()
	if workers < 1 {
		workers = 1
	}
	if workers > len(items) && len(items) > 0 {
		workers = len(items)
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
		outer:
			for {
				var index int
				select {
				case <-ctx.Done():
					break outer
				case next, ok := <-jobs:
					if !ok {
						break outer
					}
					index = next
				}
				if ctx.Err() != nil {
					break outer
				}
				if progress != nil {
					key, text := detail(items[index])
					progress.startWork(key, text)
				}
				results[index] = work(items[index])
				if progress != nil {
					key, text := detail(items[index])
					progress.finish(key, text)
				}
			}
		}()
	}
outer:
	for index := range items {
		select {
		case <-ctx.Done():
			break outer
		case jobs <- index:
		}
	}
	close(jobs)
	wg.Wait()
	return results
}

func interrupted(a *app) bool {
	if a == nil || a.ctx.Err() == nil {
		return false
	}
	renderCancellation(a)
	return true
}

func renderCancellation(a *app) {
	if a == nil {
		return
	}
	a.cancelOnce.Do(func() {
		renderStatusLine(a, a.stdout, statusSkip, "stopped", "operation cancelled")
	})
}
