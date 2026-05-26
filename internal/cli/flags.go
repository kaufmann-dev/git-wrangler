package cli

type flagSpec struct {
	name        string
	value       string
	description string
	boolean     bool
}

type flags []flagSpec

func stringFlag(name, value, description string) flagSpec {
	return flagSpec{name: name, value: value, description: description}
}

func boolFlag(name, description string) flagSpec {
	return flagSpec{name: name, description: description, boolean: true}
}
