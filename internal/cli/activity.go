package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type activityDay string

type activityResult struct {
	repo            repo
	commits         map[string]activityDay
	defaultFallback bool
	err             error
}

func runActivity(a *app, cmd *cobra.Command, args []string) int {
	year, _ := cmd.Flags().GetInt("year")
	if cmd.Flags().Changed("year") && (year < 1 || year > 9999) {
		a.error("--year must be from 1 through 9999.")
		return 1
	}
	if !requireGit(a, "activity") {
		return 1
	}
	users, _ := cmd.Flags().GetStringArray("user")
	userSet := make(map[string]struct{}, len(users))
	for _, user := range users {
		userSet[strings.ToLower(user)] = struct{}{}
	}
	all, _ := cmd.Flags().GetBool("all")
	globalScale, _ := cmd.Flags().GetBool("global-scale")
	repos, err := commandRepositoryTargets(cmd)
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}

	results := parallelReposProgress(a.ctx, repos, newProgress(a, "Scanning activity", len(repos)), func(r repo) activityResult {
		return collectActivity(a, r, all, year, userSet)
	})
	if interrupted(a) {
		return 1
	}

	days := map[activityDay]int{}
	seen := map[string]struct{}{}
	failed := 0
	for _, result := range results {
		if result.defaultFallback {
			renderWarning(a, fmt.Sprintf("%s: origin/HEAD unavailable; using current HEAD", result.repo.display))
		}
		if result.err != nil {
			renderErrorBlock(a, fmt.Sprintf("%s: could not scan activity", result.repo.display), result.err.Error())
			failed++
			continue
		}
		for hash, day := range result.commits {
			if _, ok := seen[hash]; ok {
				continue
			}
			seen[hash] = struct{}{}
			days[day]++
		}
	}

	if len(days) == 0 && year == 0 {
		fmt.Fprintln(a.stdout, "No activity found.")
		fmt.Fprintln(a.stdout)
		renderActivitySummary(a, 0, len(repos), failed)
		if failed > 0 {
			return 1
		}
		return 0
	}
	renderActivity(a, days, year, all, users, globalScale)
	fmt.Fprintln(a.stdout)
	renderActivitySummary(a, len(seen), len(repos), failed)
	if failed > 0 {
		return 1
	}
	return 0
}

func collectActivity(a *app, r repo, all bool, year int, users map[string]struct{}) activityResult {
	result := activityResult{repo: r, commits: map[string]activityDay{}}
	logArgs := []string{"log", "-z", "--format=%H%x00%aI%x00%an%x00%ae"}
	if all {
		logArgs = append(logArgs, "--exclude=refs/git-wrangler/*", "--all")
	} else {
		refs, fallback := activityDefaultRefs(a, r)
		result.defaultFallback = fallback
		logArgs = append(logArgs, refs...)
	}
	result.err = a.git.StreamStdout(a.ctx, r.dir, nil, func(output io.Reader) error {
		return parseActivityLog(output, year, users, result.commits)
	}, logArgs...)
	if result.err != nil {
		result.commits = nil
	}
	return result
}

func activityDefaultRefs(a *app, r repo) ([]string, bool) {
	originHead, err := a.git.Stdout(a.ctx, r.dir, nil, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	fallback := err != nil || !strings.HasPrefix(strings.TrimSpace(originHead), "origin/")
	refs := []string{}
	if fallback {
		refs = append(refs, "HEAD")
	} else {
		branch := strings.TrimPrefix(strings.TrimSpace(originHead), "origin/")
		refs = append(refs, preferredActivityRef(a, r.dir, branch))
	}
	if ref := existingActivityRef(a, r.dir, "gh-pages"); ref != "" && !containsString(refs, ref) {
		refs = append(refs, ref)
	}
	return refs, fallback
}

func preferredActivityRef(a *app, dir, branch string) string {
	if a.git.VerifyRef(a.ctx, dir, "refs/heads/"+branch) {
		return "refs/heads/" + branch
	}
	return "refs/remotes/origin/" + branch
}

func existingActivityRef(a *app, dir, branch string) string {
	local := "refs/heads/" + branch
	if a.git.VerifyRef(a.ctx, dir, local) {
		return local
	}
	remote := "refs/remotes/origin/" + branch
	if a.git.VerifyRef(a.ctx, dir, remote) {
		return remote
	}
	return ""
}

func parseActivityLog(output io.Reader, year int, users map[string]struct{}, commits map[string]activityDay) error {
	reader := bufio.NewReader(output)
	for {
		hash, err := readNULField(reader)
		if errors.Is(err, io.EOF) && hash == "" {
			return nil
		}
		if err != nil {
			return err
		}
		dateText, err := readNULField(reader)
		if err != nil {
			return err
		}
		name, err := readNULField(reader)
		if err != nil {
			return err
		}
		email, err := readNULField(reader)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if len(users) > 0 {
			_, nameMatch := users[strings.ToLower(name)]
			_, emailMatch := users[strings.ToLower(email)]
			if !nameMatch && !emailMatch {
				if errors.Is(err, io.EOF) {
					return nil
				}
				continue
			}
		}
		date, parseErr := time.Parse(time.RFC3339, dateText)
		if parseErr != nil {
			return fmt.Errorf("invalid author date for %s: %w", hash, parseErr)
		}
		date = date.UTC()
		if year == 0 || date.Year() == year {
			commits[hash] = activityDay(date.Format("2006-01-02"))
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
	}
}

func readNULField(reader *bufio.Reader) (string, error) {
	value, err := reader.ReadString(0)
	return strings.TrimSuffix(value, "\x00"), err
}

func renderActivity(a *app, days map[activityDay]int, requestedYear int, all bool, users []string, globalScale bool) {
	years := activityYears(days, requestedYear)
	globalMax := 0
	if globalScale {
		globalMax = maxActivityCount(days, 0)
	}
	fmt.Fprintln(a.stdout, "Activity")
	scope := "default branch + gh-pages"
	if all {
		scope = "all refs"
	}
	authors := "all authors"
	if len(users) > 0 {
		authors = "authors: " + strings.Join(users, ", ")
	}
	fmt.Fprintf(a.stdout, "Scope: %s, %s, UTC\n", scope, authors)
	if globalScale {
		fmt.Fprintf(a.stdout, "Scale: global, Max: %d/day\n", globalMax)
	} else {
		fmt.Fprintln(a.stdout, "Scale: per year")
	}
	for _, year := range years {
		fmt.Fprintln(a.stdout)
		renderActivityYear(a, days, year, globalMax)
	}
	fmt.Fprintln(a.stdout)
	renderActivityLegend(a)
}

func activityYears(days map[activityDay]int, requestedYear int) []int {
	if requestedYear != 0 {
		return []int{requestedYear}
	}
	set := map[int]struct{}{}
	for day := range days {
		year, _ := strconv.Atoi(string(day)[:4])
		set[year] = struct{}{}
	}
	years := make([]int, 0, len(set))
	for year := range set {
		years = append(years, year)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(years)))
	return years
}

func renderActivityYear(a *app, days map[activityDay]int, year, globalMax int) {
	yearMax := maxActivityCount(days, year)
	scaleMax := yearMax
	if globalMax > 0 {
		scaleMax = globalMax
	}
	total := 0
	for day, count := range days {
		if strings.HasPrefix(string(day), strconv.Itoa(year)+"-") {
			total += count
		}
	}
	fmt.Fprintf(a.stdout, "%d  %d commits  Max: %d/day\n", year, total, scaleMax)

	first := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	start := first.AddDate(0, 0, -int(first.Weekday()))
	last := time.Date(year, time.December, 31, 0, 0, 0, 0, time.UTC)
	end := last.AddDate(0, 0, 6-int(last.Weekday()))
	weeks := int(end.Sub(start).Hours()/24)/7 + 1

	heading := make([]byte, weeks)
	for i := range heading {
		heading[i] = ' '
	}
	for month := time.January; month <= time.December; month++ {
		monthStart := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
		week := int(monthStart.Sub(start).Hours()/24) / 7
		label := month.String()[:3]
		for i := range label {
			if week+i < len(heading) {
				heading[week+i] = label[i]
			}
		}
	}
	fmt.Fprintf(a.stdout, "      %s\n", string(heading))
	weekdays := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	for weekday, label := range weekdays {
		fmt.Fprintf(a.stdout, "%s   ", label)
		for week := 0; week < weeks; week++ {
			date := start.AddDate(0, 0, week*7+weekday)
			if date.Year() != year {
				fmt.Fprint(a.stdout, " ")
				continue
			}
			count := days[activityDay(date.Format("2006-01-02"))]
			fmt.Fprint(a.stdout, activityCell(a, activityLevel(count, scaleMax)))
		}
		fmt.Fprintln(a.stdout)
	}
}

func maxActivityCount(days map[activityDay]int, year int) int {
	max := 0
	for day, count := range days {
		if year != 0 && !strings.HasPrefix(string(day), strconv.Itoa(year)+"-") {
			continue
		}
		if count > max {
			max = count
		}
	}
	return max
}

func activityLevel(count, max int) int {
	if count == 0 || max == 0 {
		return 0
	}
	level := (count*4 + max - 1) / max
	if level > 4 {
		return 4
	}
	return level
}

func activityCell(a *app, level int) string {
	if a.ui.Reset == "" {
		return ".1234"[level : level+1]
	}
	colors := []string{
		"\033[48;2;22;27;34m",
		"\033[48;2;14;68;41m",
		"\033[48;2;0;109;50m",
		"\033[48;2;38;166;65m",
		"\033[48;2;57;211;83m",
	}
	return colors[level] + " " + a.ui.Reset
}

func renderActivityLegend(a *app) {
	fmt.Fprint(a.stdout, "Less  ")
	for level := 0; level <= 4; level++ {
		if level > 0 {
			fmt.Fprint(a.stdout, " ")
		}
		fmt.Fprint(a.stdout, activityCell(a, level))
	}
	fmt.Fprintln(a.stdout, "  More")
}

func renderActivitySummary(a *app, commits, repositories, failed int) {
	renderSummary(a,
		summaryCount{label: "commits", value: commits, color: a.ui.Green},
		summaryCount{label: "repositories", value: repositories, color: a.ui.Cyan},
		summaryCount{label: "failed", value: failed, color: a.ui.Red},
	)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
