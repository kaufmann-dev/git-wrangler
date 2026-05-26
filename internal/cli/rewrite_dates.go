package cli

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kaufmann-dev/git-wrangler/internal/git"
	"github.com/spf13/cobra"
)

func runRewriteDates(a *app, cmd *cobra.Command, args []string) int {
	startDate, _ := cmd.Flags().GetString("start-date")
	endDate, _ := cmd.Flags().GetString("end-date")
	confirmed, _ := cmd.Flags().GetBool("confirm")
	if startDate != "" && !validDate(startDate) {
		a.error("--start-date must be in YYYY-MM-DD format.")
		return 1
	}
	if endDate != "" && !validDate(endDate) {
		a.error("--end-date must be in YYYY-MM-DD format.")
		return 1
	}
	if !requireGit(a, "rewrite-dates") {
		return 1
	}
	filterCmd, ok := filterRepoCommand(a, "rewrite-dates")
	if !ok {
		return 1
	}
	repos, err := findGitRepositories(".")
	if err != nil {
		a.error(err.Error())
		return 1
	}
	if len(repos) == 0 {
		return noRepos(a)
	}
	status := 0
	for _, r := range repos {
		if _, err := runCapture(r.dir, nil, "git", "rev-parse", "HEAD"); err != nil {
			fmt.Fprintf(a.stdout, "%s%s has no commits. Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		fmt.Fprintf(a.stdout, "%sProcessing %s...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
		countOut, err := runStdout(r.dir, nil, "git", "rev-list", "--all", "--count")
		if err != nil {
			fmt.Fprintf(a.stderr, "%sError: Could not count commits for %s:\n%s%s\n\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
			status = 1
			continue
		}
		count, err := strconv.Atoi(strings.TrimSpace(countOut))
		if err != nil {
			fmt.Fprintf(a.stderr, "%sError: malformed commit count for %s: %q%s\n", a.ui.Red, r.display, strings.TrimSpace(countOut), a.ui.Reset)
			status = 1
			continue
		}
		if count < 2 {
			fmt.Fprintf(a.stdout, "%s%s has fewer than 2 commits. Skipping...%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		startEpoch := int64(0)
		endEpoch := int64(0)
		if startDate != "" {
			startEpoch = parseLocalDate(startDate)
		} else {
			startEpoch, err = firstCommitEpoch(r.dir, "--reverse")
			if err != nil {
				fmt.Fprintf(a.stderr, "%sError: could not read start timestamp for %s: %s%s\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
				status = 1
				continue
			}
		}
		if endDate != "" {
			endEpoch = parseLocalDate(endDate)
		} else {
			endEpoch, err = firstCommitEpoch(r.dir)
			if err != nil {
				fmt.Fprintf(a.stderr, "%sError: could not read end timestamp for %s: %s%s\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
				status = 1
				continue
			}
		}
		if startEpoch >= endEpoch {
			fmt.Fprintf(a.stderr, "%sError: start date must be before end date in %s.%s\n", a.ui.Red, r.display, a.ui.Reset)
			status = 1
			continue
		}
		remoteURL := originURL(r.dir)
		tzOffset, err := dominantTimezoneOffset(r.dir)
		if err != nil {
			fmt.Fprintf(a.stderr, "%sError: could not read time zones for %s: %s%s\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
			status = 1
			continue
		}
		commits, err := readCommitTimes(r.dir)
		if err != nil {
			fmt.Fprintf(a.stderr, "%sError: could not read commit timestamps for %s: %s%s\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
			status = 1
			continue
		}
		mapping := distributeCommitTimes(commits, startEpoch, endEpoch)
		fmt.Fprintln(a.stdout, "Commit summary (old -> new):")
		fmt.Fprintln(a.stdout, strings.Repeat("-", 70))
		for _, c := range commits {
			fmt.Fprintf(a.stdout, "  %s  %s -> %s\n", prefix(c.hash, 8), formatEpoch(c.epoch, tzOffset), formatEpoch(mapping[c.hash], tzOffset))
		}
		fmt.Fprintf(a.stderr, "%s\nWARNING: This operation rewrites Git history. A force push will be required to update any remote.%s\n\n", a.ui.Red, a.ui.Reset)
		if !confirmed && !confirm(a, "Proceed with rewrite for "+r.display+"?") {
			fmt.Fprintf(a.stdout, "%sSkipping %s.%s\n", a.ui.Yellow, r.display, a.ui.Reset)
			continue
		}
		callback, err := writeDateCallback(mapping, tzOffset)
		if err != nil {
			fmt.Fprintf(a.stderr, "%sError: timestamp generation failed for %s:\n%s%s\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
			status = 1
			continue
		}
		out, err := runFilterRepo(r.dir, filterCmd, []string{"--partial", "--commit-callback", callback, "--force"}, nil)
		_ = os.Remove(callback)
		if err == nil {
			fmt.Fprintf(a.stdout, "%sSuccessfully rewrote commit dates for %s%s\n", a.ui.Green, r.display, a.ui.Reset)
			if err := restoreOrigin(r.dir, remoteURL); err != nil {
				fmt.Fprintf(a.stderr, "%sWarning: Date rewrite completed for %s, but origin could not be restored:\n%s%s\n\n", a.ui.Red, r.display, err.Error(), a.ui.Reset)
				status = 1
			}
		} else {
			fmt.Fprintf(a.stderr, "%sError: rewrite failed for %s:\n%s%s\n", a.ui.Red, r.display, out, a.ui.Reset)
			status = 1
		}
	}
	return status
}

func validDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func parseLocalDate(s string) int64 {
	t, _ := time.ParseInLocation("2006-01-02", s, time.Local)
	return t.Unix()
}

type commitTime struct {
	hash  string
	epoch int64
}

func firstCommitEpoch(dir string, flags ...string) (int64, error) {
	args := append([]string{"log", "--all"}, flags...)
	args = append(args, "--format=%at")
	if !stringSliceContains(flags, "--reverse") {
		args = append(args, "-1")
	}
	out, err := runStdout(dir, nil, "git", args...)
	if err != nil {
		return 0, err
	}
	value := strings.TrimSpace(firstLine(out))
	if value == "" {
		return 0, fmt.Errorf("empty timestamp")
	}
	epoch, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("malformed timestamp %q", value)
	}
	return epoch, nil
}

func stringSliceContains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func readCommitTimes(dir string) ([]commitTime, error) {
	out, err := runStdout(dir, nil, "git", "log", "--all", "--reverse", "--format=%H %at")
	if err != nil {
		return nil, err
	}
	var commits []commitTime
	for _, line := range splitLines(out) {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("malformed commit timestamp line %q", line)
		}
		epoch, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("malformed timestamp %q for %s", fields[1], fields[0])
		}
		commits = append(commits, commitTime{hash: fields[0], epoch: epoch})
	}
	return commits, nil
}

func dominantTimezoneOffset(dir string) (string, error) {
	out, err := runStdout(dir, nil, "git", "log", "--all", "--format=%ai")
	if err != nil {
		return "", err
	}
	counts := map[string]int{}
	for _, line := range splitLines(out) {
		if len(line) >= 5 {
			offset := line[len(line)-5:]
			if timezoneOffsetRe.MatchString(offset) {
				counts[offset]++
			}
		}
	}
	best := ""
	bestCount := 0
	for offset, count := range counts {
		if count > bestCount {
			best = offset
			bestCount = count
		}
	}
	if best != "" {
		return best, nil
	}
	return time.Now().Format("-0700"), nil
}

func distributeCommitTimes(commits []commitTime, startEpoch, endEpoch int64) map[string]int64 {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	n := len(commits)
	if n == 0 {
		return map[string]int64{}
	}
	totalRange := float64(endEpoch - startEpoch)
	if totalRange <= 0 {
		totalRange = 86400
	}
	slotWidth := totalRange / float64(n)
	timestamps := make([]int64, n)
	for i := range commits {
		slotStart := float64(startEpoch) + float64(i)*slotWidth
		slotCenter := slotStart + slotWidth/2
		jitter := (rng.Float64()*0.8 - 0.4) * slotWidth
		raw := slotCenter + jitter
		dayStart := int64(raw/86400) * 86400
		hour := sampleBimodalHour(rng)
		ts := dayStart + int64(hour)*3600 + int64(rng.Intn(60))*60 + int64(rng.Intn(60))
		if isWeekend(ts) && rng.Float64() < 0.65 {
			wd := weekdayFromEpoch(ts)
			if wd == 2 {
				ts = dayStart - 86400 + int64(18+rng.Intn(5))*3600 + int64(rng.Intn(60))*60 + int64(rng.Intn(60))
			} else {
				ts = dayStart + 86400 + int64(7+rng.Intn(3))*3600 + int64(rng.Intn(60))*60 + int64(rng.Intn(60))
			}
		}
		timestamps[i] = ts
	}
	dayGroups := map[int64][]int{}
	for i, ts := range timestamps {
		dayGroups[ts/86400] = append(dayGroups[ts/86400], i)
	}
	for day, indices := range dayGroups {
		if len(indices) < 2 {
			continue
		}
		rng.Shuffle(len(indices), func(i, j int) { indices[i], indices[j] = indices[j], indices[i] })
		spacing := int64((25 + rng.Intn(66)) * 60)
		latestStart := 22.0 - float64(len(indices)-1)*float64(spacing)/3600.0
		startHour := 7.0
		if latestStart > 7.0 {
			startHour = 7.0 + rng.Float64()*(latestStart-7.0)
		}
		current := int64(startHour * 3600)
		for _, idx := range indices {
			timestamps[idx] = day*86400 + current + int64(rng.Intn(60))
			current += spacing
		}
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })
	mapping := map[string]int64{}
	for i, c := range commits {
		mapping[c.hash] = timestamps[i]
	}
	return mapping
}

func sampleBimodalHour(rng *rand.Rand) int {
	peak := 10.0
	if rng.Float64() >= 0.5 {
		peak = 15.0
	}
	u1 := rng.Float64()
	if u1 == 0 {
		u1 = 1e-10
	}
	u2 := rng.Float64()
	z := math.Sqrt(-2.0*math.Log(u1)) * math.Cos(2.0*math.Pi*u2)
	hour := peak + 2.0*z
	if hour < 7 {
		hour = 7
	}
	if hour > 22 {
		hour = 22
	}
	return int(hour)
}

func weekdayFromEpoch(ts int64) int64 {
	return (ts/86400 + 4) % 7
}

func isWeekend(ts int64) bool {
	wd := weekdayFromEpoch(ts)
	return wd == 0 || wd == 6
}

func formatEpoch(epoch int64, offset string) string {
	sign := 1
	if strings.HasPrefix(offset, "-") {
		sign = -1
	}
	hours, _ := strconv.Atoi(offset[1:3])
	minutes, _ := strconv.Atoi(offset[3:5])
	loc := time.FixedZone(offset, sign*(hours*3600+minutes*60))
	return time.Unix(epoch, 0).In(loc).Format("2006-01-02 15:04:05 ") + offset
}

func writeDateCallback(mapping map[string]int64, tzOffset string) (string, error) {
	f, err := os.CreateTemp("", "git-wrangler-date-callback-*")
	if err != nil {
		return "", err
	}
	defer f.Close()
	fmt.Fprintln(f, "mapping = {}")
	keys := make([]string, 0, len(mapping))
	for key := range mapping {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(f, "mapping[%s] = %s\n", git.PythonBytesLiteral(key), git.PythonBytesLiteral(fmt.Sprintf("%d %s", mapping[key], tzOffset)))
	}
	fmt.Fprintln(f, "if commit.original_id in mapping:")
	fmt.Fprintln(f, "    commit.author_date = mapping[commit.original_id]")
	fmt.Fprintln(f, "    commit.committer_date = mapping[commit.original_id]")
	return f.Name(), nil
}
