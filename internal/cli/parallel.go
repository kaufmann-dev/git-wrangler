package cli

import (
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

func parallelRepos[T any](repos []repo, inspect func(repo) T) []T {
	return parallelReposWithWorkers(repos, readOnlyWorkerCount(len(repos)), inspect)
}

func parallelReposProgress[T any](repos []repo, progress *progress, inspect func(repo) T) []T {
	return parallelReposWithWorkersProgress(repos, readOnlyWorkerCount(len(repos)), progress, inspect)
}

func parallelGitMutations[T any](repos []repo, mutate func(repo) T) []T {
	return parallelReposWithWorkers(repos, gitMutationWorkerCount(len(repos)), mutate)
}

func parallelGitMutationsProgress[T any](repos []repo, progress *progress, mutate func(repo) T) []T {
	return parallelReposWithWorkersProgress(repos, gitMutationWorkerCount(len(repos)), progress, mutate)
}

func parallelReposWithWorkers[T any](repos []repo, workers int, inspect func(repo) T) []T {
	return parallelReposWithWorkersProgress(repos, workers, nil, inspect)
}

func parallelReposWithWorkersProgress[T any](repos []repo, workers int, progress *progress, inspect func(repo) T) []T {
	return parallelItemsWithWorkersProgress(repos, workers, progress, func(r repo) (string, string) {
		return r.display, r.display
	}, inspect)
}

func parallelItemsWithWorkersProgress[T any, R any](items []T, workers int, progress *progress, detail func(T) (string, string), work func(T) R) []R {
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
			for index := range jobs {
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
	for index := range items {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	return results
}
