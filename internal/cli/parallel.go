package cli

import (
	"runtime"
	"sync"
)

func readOnlyWorkerCount(repoCount int) int {
	workers := runtime.NumCPU()
	if workers > 32 {
		workers = 32
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
	results := make([]T, len(repos))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for i := 0; i < readOnlyWorkerCount(len(repos)); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				results[index] = inspect(repos[index])
			}
		}()
	}
	for index := range repos {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	return results
}
