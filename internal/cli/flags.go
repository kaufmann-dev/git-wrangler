package cli

type flagSpec struct {
	name        string
	stringValue string
	intValue    int
	description string
	kind        string
}

type flags []flagSpec

func stringFlag(name, value, description string) flagSpec {
	return flagSpec{name: name, stringValue: value, description: description, kind: "string"}
}

func stringArrayFlag(name, description string) flagSpec {
	return flagSpec{name: name, description: description, kind: "stringArray"}
}

func boolFlag(name, description string) flagSpec {
	return flagSpec{name: name, description: description, kind: "bool"}
}

func intFlag(name string, value int, description string) flagSpec {
	return flagSpec{name: name, intValue: value, description: description, kind: "int"}
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
