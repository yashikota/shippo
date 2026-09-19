package daemon

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
)

// Init lets the user choose an allowlist without requiring BPF or Tailscale privileges.
func Init(input io.Reader, output io.Writer) error {
	ports := make([]int, 0)
	for port := range detectCurrentPorts() {
		ports = append(ports, port)
	}
	slices.Sort(ports)
	return initConfig(input, output, ports)
}

func initConfig(input io.Reader, output io.Writer, ports []int) error {
	if os.Getenv("SHIPPO_PORTS") != "" {
		return fmt.Errorf("SHIPPO_PORTS overrides the config file; unset it before running shippo init")
	}
	path := configFilePath()
	if path == "" {
		return fmt.Errorf("cannot determine config path")
	}
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(output, "Existing config: %s (replaced only if you save)\n", path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read config: %w", err)
	}

	fmt.Fprintln(output, "Choose which localhost ports to expose through Tailscale.")
	fmt.Fprintln(output, "Nothing is selected by default.")
	scanner := bufio.NewScanner(input)
	ask := func(prompt string) (string, error) {
		fmt.Fprint(output, prompt)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", fmt.Errorf("read selection: %w", err)
			}
			return "", fmt.Errorf("initialization cancelled: input ended before confirmation")
		}
		return strings.TrimSpace(scanner.Text()), nil
	}

	raw := make([]string, 0)
	if len(ports) > 0 {
		fmt.Fprintln(output, "Currently listening:")
		for i, port := range ports {
			fmt.Fprintf(output, "  %d) %d\n", i+1, port)
		}
		for {
			line, err := ask("Select candidate numbers, separated by commas (*: all ports, Enter: none): ")
			if err != nil {
				return err
			}
			selected, err := selectPorts(line, ports)
			if err != nil {
				fmt.Fprintln(output, err)
				continue
			}
			raw = append(raw, selected...)
			break
		}
	} else {
		fmt.Fprintln(output, "No localhost ports are currently listening.")
	}

	for !slices.Contains(raw, "*") {
		line, err := ask("Additional ports or ranges, separated by commas (*: all, Enter: none): ")
		if err != nil {
			return err
		}
		additional, err := validatePortSelection(line)
		if err != nil {
			fmt.Fprintln(output, err)
			continue
		}
		for _, spec := range additional {
			if !slices.Contains(raw, spec) {
				raw = append(raw, spec)
			}
		}
		break
	}

	allowed := strings.Join(raw, ", ")
	if len(raw) == 0 {
		allowed = "none"
	} else if slices.Contains(raw, "*") {
		raw = []string{"*"}
		allowed = "ALL localhost ports (*)"
	}
	fmt.Fprintf(output, "Allowed ports: %s\nConfig: %s\n", allowed, path)
	answer, err := ask("Save this configuration? [y/N]: ")
	if err != nil {
		return err
	}
	if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
		fmt.Fprintln(output, "Cancelled; configuration unchanged.")
		return nil
	}
	matchers, _ := parsePortSpecs(strings.Join(raw, ","))
	if err := saveConfigFile(&Config{matchers: matchers, raw: raw}); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	fmt.Fprintln(output, "Configuration saved. Start shippo to watch the selected ports.")
	return nil
}

func selectPorts(line string, ports []int) ([]string, error) {
	if line == "*" {
		return []string{"*"}, nil
	}
	var selected []string
	if line == "" {
		return selected, nil
	}
	for _, part := range strings.Split(line, ",") {
		index, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || index < 1 || index > len(ports) {
			return nil, fmt.Errorf("invalid candidate %q; choose numbers from 1 to %d", part, len(ports))
		}
		port := strconv.Itoa(ports[index-1])
		if !slices.Contains(selected, port) {
			selected = append(selected, port)
		}
	}
	return selected, nil
}

func validatePortSelection(line string) ([]string, error) {
	var raw []string
	if line == "" {
		return raw, nil
	}
	for _, part := range strings.Split(line, ",") {
		matchers, _ := parsePortSpecs(part)
		if len(matchers) != 1 {
			return nil, fmt.Errorf("invalid port specification %q; use ports 1-65535, ranges, or *", part)
		}
		raw = append(raw, matchers[0].String())
	}
	return raw, nil
}
