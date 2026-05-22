package daemon

import (
	"fmt"
	"strings"
)

// Rules prints the current port allow rules.
func Rules() {
	config := LoadConfig()
	raw := config.GetRaw()

	if len(raw) == 0 {
		fmt.Println("No rules configured.")
		fmt.Printf("Config: %s\n", configFilePath())
		return
	}

	fmt.Println("Port allow rules:")
	for _, r := range raw {
		matchers, _ := parsePortSpecs(r)
		desc := ""
		if len(matchers) > 0 {
			switch matchers[0].(type) {
			case wildcardMatcher:
				desc = "(all ports)"
			case exactMatcher:
				desc = "(single port)"
			case rangeMatcher:
				desc = "(range)"
			}
		}
		fmt.Printf("  %s  %s\n", r, desc)
	}
	fmt.Printf("\nConfig: %s\n", configFilePath())
}

// Status prints current listening ports and their serve status.
func Status() {
	config := LoadConfig()
	ports := detectCurrentPorts()

	fmt.Println("Allowed:", config.String())
	fmt.Println()

	if len(ports) == 0 {
		fmt.Println("No localhost ports currently listening.")
		return
	}

	fmt.Println("Listening on localhost:")
	for port := range ports {
		status := "  (not in allow list)"
		if config.IsAllowed(port) {
			status = fmt.Sprintf("  → https://%s:%d", getMachineName(), port)
		}
		fmt.Printf("  :%d%s\n", port, status)
	}
}

// Add adds port specs to the config file.
func Add(specs []string) error {
	config := LoadConfig()
	existing := config.GetRaw()

	added := []string{}
	for _, arg := range specs {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		found := false
		for _, e := range existing {
			if e == arg {
				found = true
				break
			}
		}
		if !found {
			existing = append(existing, arg)
			added = append(added, arg)
		}
	}

	if len(added) == 0 {
		fmt.Println("No new specs to add.")
		return nil
	}

	matchers, raw := parsePortSpecs(strings.Join(existing, ","))
	config.SetMatchers(matchers, raw)

	if err := saveConfigFile(config); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("Added: %s\n", strings.Join(added, ", "))
	fmt.Printf("Allowed: %s\n", config.String())
	return nil
}

// Remove removes port specs from the config file.
func Remove(specs []string) error {
	config := LoadConfig()
	existing := config.GetRaw()

	toRemove := map[string]bool{}
	for _, arg := range specs {
		toRemove[strings.TrimSpace(arg)] = true
	}

	remaining := []string{}
	removed := []string{}
	for _, e := range existing {
		if toRemove[e] {
			removed = append(removed, e)
		} else {
			remaining = append(remaining, e)
		}
	}

	if len(removed) == 0 {
		fmt.Println("No specs to remove.")
		return nil
	}

	matchers, raw := parsePortSpecs(strings.Join(remaining, ","))
	config.SetMatchers(matchers, raw)

	if err := saveConfigFile(config); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("Removed: %s\n", strings.Join(removed, ", "))
	fmt.Printf("Allowed: %s\n", config.String())
	return nil
}
