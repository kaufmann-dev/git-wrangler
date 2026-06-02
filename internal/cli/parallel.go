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

func parallelReposProgress[T any](repos []repo, progress *progress, inspect func(repo) T) []T {
	return parallelReposWithWorkersProgress(repos, readOnlyWorkerCount(len(repos)), progress, inspect)
}

func parallelGitMutations[T any](repos []repo, mutate func(repo) T) []T {
	return parallelReposWithWorkers(repos, gitMutationWorkerCount(len(repos)), mutate)
}

func parallelGitMutationsProgress[T any](repos []repo, progress *progress, mutate func(repo) T) []T {
	return parallelReposWithWorkersProgress(repos, gitMutationWorkerCount(len(repos)), progress, mutate)
}

func parallelHistoryRewrites[T any](repos []repo, rewrite func(repo) T) []T {
	return parallelReposWithWorkers(repos, historyRewriteWorkerCount(len(repos)), rewrite)
}

func parallelReposWithWorkers[T any](repos []repo, workers int, inspect func(repo) T) []T {
	return parallelReposWithWorkersProgress(repos, workers, nil, inspect)
}

func parallelReposWithWorkersProgress[T any](repos []repo, workers int, progress *progress, inspect func(repo) T) []T {
	results := make([]T, len(repos))
	jobs := make(chan int)
	var wg sync.WaitGroup
	defer progress.done()
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
				if progress != nil {
					progress.start(repos[index].display)
				}
				results[index] = inspect(repos[index])
				if progress != nil {
					progress.finish(repos[index].display, repos[index].display)
				}
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
