package cli

type flagKind int

const (
	flagKindString flagKind = iota
	flagKindStringArray
	flagKindBool
	flagKindInt
)

type flagSpec struct {
	name        string
	shorthand   string
	stringValue string
	intValue    int
	description string
	kind        flagKind
}

type flags []flagSpec

func stringFlag(name, value, description string) flagSpec {
	return flagSpec{name: name, stringValue: value, description: description, kind: flagKindString}
}

func stringArrayFlag(name, description string) flagSpec {
	return flagSpec{name: name, description: description, kind: flagKindStringArray}
}

func boolFlag(name, description string) flagSpec {
	return flagSpec{name: name, description: description, kind: flagKindBool}
}

func intFlag(name string, value int, description string) flagSpec {
	return flagSpec{name: name, intValue: value, description: description, kind: flagKindInt}
}

func repoFlag() flagSpec {
	return stringFlag("repo", "", "Exact repository directory to target.")
}

func jsonFlag() flagSpec {
	return boolFlag("json", "Emit one JSON document.")
}

func noFetchFlag() flagSpec {
	return boolFlag("no-fetch", "Use local remote-tracking refs without fetching first.")
}

func yesFlagSpec() flagSpec {
	spec := boolFlag("yes", "Skip confirmation prompts.")
	spec.shorthand = "y"
	return spec
}

func targetFlags() flags {
	return flags{repoFlag()}
}

func jsonFlags() flags {
	return flags{jsonFlag()}
}

func fetchControlFlags() flags {
	return flags{noFetchFlag()}
}

func confirmationFlags() flags {
	return flags{yesFlagSpec()}
}

func aiRequestFlags() flags {
	return flags{
		intFlag("rpm", 300, "Maximum API requests to start per minute."),
		intFlag("concurrency", 8, "Maximum in-flight API requests."),
		intFlag("timeout", 90, "API timeout in seconds."),
	}
}

func rewriteDateBoundFlags() flags {
	return flags{
		stringFlag("rewrite-after", "", "Rewrite commits with current author dates on or after YYYY-MM-DD."),
		stringFlag("rewrite-before", "", "Rewrite commits with current author dates before YYYY-MM-DD."),
	}
}

func joinFlags(groups ...flags) flags {
	var result flags
	for _, group := range groups {
		result = append(result, group...)
	}
	return result
}
