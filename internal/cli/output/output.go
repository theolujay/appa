package output

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

// PrintTable prints a formatted table to standard output using a tabwriter.
//
// Example output:
//
//	`
//	NAME       HOST                    STATUS
//	personal   root@203.0.113.10       done
//	staging    root@198.51.100.20      host set
//	`
func PrintTable(header []string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, strings.Join(header, "\t"))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush()
}

// Check prints a status check line. It displays a checkmark (✓) if ok is true,
// and a cross (✗) otherwise. The label supports fmt-style formatting.
//
// Example output:
//
//	`
//	✓ SSH target
//	✗ connection timed out
//	`
func Check(label string, ok bool, args ...any) {
	s := fmt.Sprintf(label, args...)
	if ok {
		fmt.Printf("✓ %s\n", s)
	} else {
		fmt.Printf("✗ %s\n", s)
	}
}

// Warn prints a warning message to standard output, prefixed with "! ".
//
// Example output:
//
//	`! Disk usage is high: 90%`
func Warn(format string, args ...any) {
	fmt.Printf("! "+format+"\n", args...)
}

// Section prints a section header with an underline for visual separation in CLI output.
//
// Example output:
//
//	`
//	Deployment Status
//	-----------------
//	`
func Section(format string, args ...any) {
	title := fmt.Sprintf(format, args...)
	fmt.Println()
	fmt.Println(title)
	fmt.Println(strings.Repeat("-", len(title)))
}

// Error prints a formatted error message to standard error, prefixed with "Error: ".
//
// Example output:
//
//	`Error: Failed to connect: connection refused`
func Error(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
}

// Success prints a formatted success message to standard output.
//
// Example output:
//
//	`Instance profile "personal" created`
func Success(format string, args ...any) {
	fmt.Fprintf(os.Stdout, format+"\n", args...)
}
