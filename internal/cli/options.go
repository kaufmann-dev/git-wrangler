package cli

import "github.com/spf13/cobra"

type targetOptions struct {
	repo string
}

type jsonOptions struct {
	enabled bool
}

type fetchOptions struct {
	noFetch bool
}

type confirmationOptions struct {
	yes bool
}

type aiRequestOptions struct {
	rpm         int
	concurrency int
	timeout     int
	body        bool
}

type rewriteBoundOptions struct {
	bounds currentRewriteDateBounds
}

func targetOptionsFromCommand(cmd *cobra.Command) targetOptions {
	return targetOptions{repo: stringFlagValue(cmd, "repo")}
}

func (opts targetOptions) repositories() ([]repo, error) {
	return resolveRepositoryTargets(opts.repo)
}

func jsonOptionsFromCommand(cmd *cobra.Command) jsonOptions {
	return jsonOptions{enabled: boolFlagValue(cmd, "json")}
}

func fetchOptionsFromCommand(cmd *cobra.Command) fetchOptions {
	return fetchOptions{noFetch: boolFlagValue(cmd, "no-fetch")}
}

func confirmationOptionsFromCommand(cmd *cobra.Command) confirmationOptions {
	return confirmationOptions{yes: boolFlagValue(cmd, "yes")}
}

func aiRequestOptionsFromCommand(cmd *cobra.Command) aiRequestOptions {
	return aiRequestOptions{
		rpm:         intFlagValue(cmd, "rpm"),
		concurrency: intFlagValue(cmd, "concurrency"),
		timeout:     intFlagValue(cmd, "timeout"),
		body:        boolFlagValue(cmd, "body"),
	}
}

func rewriteBoundOptionsFromCommand(cmd *cobra.Command) (rewriteBoundOptions, error) {
	bounds, err := parseCurrentRewriteDateBounds(stringFlagValue(cmd, "rewrite-after"), stringFlagValue(cmd, "rewrite-before"))
	return rewriteBoundOptions{bounds: bounds}, err
}

func stringFlagValue(cmd *cobra.Command, name string) string {
	if cmd == nil || cmd.Flags().Lookup(name) == nil {
		return ""
	}
	value, _ := cmd.Flags().GetString(name)
	return value
}

func stringArrayFlagValues(cmd *cobra.Command, name string) []string {
	if cmd == nil || cmd.Flags().Lookup(name) == nil {
		return nil
	}
	values, _ := cmd.Flags().GetStringArray(name)
	return values
}

func boolFlagValue(cmd *cobra.Command, name string) bool {
	if cmd == nil || cmd.Flags().Lookup(name) == nil {
		return false
	}
	value, _ := cmd.Flags().GetBool(name)
	return value
}

func intFlagValue(cmd *cobra.Command, name string) int {
	if cmd == nil || cmd.Flags().Lookup(name) == nil {
		return 0
	}
	value, _ := cmd.Flags().GetInt(name)
	return value
}

func flagChanged(cmd *cobra.Command, name string) bool {
	return cmd != nil && cmd.Flags().Lookup(name) != nil && cmd.Flags().Changed(name)
}
