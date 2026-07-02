package cli

import "fmt"

func validatePositiveIntFlag(name string, value int) error {
	if value <= 0 {
		return fmt.Errorf("--%s must be a positive integer.", name)
	}
	return nil
}

func validateNonNegativeIntFlag(name string, value int) error {
	if value < 0 {
		return fmt.Errorf("--%s must be 0 or greater.", name)
	}
	return nil
}

func validateMaxIntFlag(name string, value, max int) error {
	if value > max {
		return fmt.Errorf("--%s must be %d or less.", name, max)
	}
	return nil
}

func validateDateFlag(name, value string) error {
	if value != "" && !validDate(value) {
		return fmt.Errorf("--%s must be in YYYY-MM-DD format.", name)
	}
	return nil
}

func validateStringEnum(value string, allowed ...string) bool {
	for _, allowedValue := range allowed {
		if value == allowedValue {
			return true
		}
	}
	return false
}
