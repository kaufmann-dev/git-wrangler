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

func historyRewriteWorkerCount(repoCount int) int {
	return cappedWorkerCount(repoCount, 1)
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

func parallelGitMutations[T any](repos []repo, mutate func(repo) T) []T {
	return parallelReposWithWorkers(repos, gitMutationWorkerCount(len(repos)), mutate)
}

func parallelHistoryRewrites[T any](repos []repo, rewrite func(repo) T) []T {
	return parallelReposWithWorkers(repos, historyRewriteWorkerCount(len(repos)), rewrite)
}

func parallelReposWithWorkers[T any](repos []repo, workers int, inspect func(repo) T) []T {
	results := make([]T, len(repos))
	jobs := make(chan int)
	var wg sync.WaitGroup
	if workers < 1 {
		workers = 1
	}
	if workers > len(repos) && len(repos) > 0 {
		workers = len(repos)
	}
	for i := 0; i < workers; i++ {
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
