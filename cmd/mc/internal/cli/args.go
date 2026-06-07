package cli

import (
	"fmt"
	"strings"
)

// valueFlags take a value; every other recognised --flag is boolean.
var valueFlags = map[string]bool{
	"uri":     true,
	"device":  true,
	"as":      true,
	"baud":    true,
	"lines":   true,
	"n":       true,
	"sort":    true,
	"timeout": true,
	"type":    true,
	"within":  true,
	"route":   true,
}

// parsedArgs holds flags and positional arguments extracted from anywhere in
// the command line, so that `mc --device x status` and `mc status --json`
// both work.
type parsedArgs struct {
	flags       map[string]string
	bools       map[string]bool
	positionals []string
}

func (p parsedArgs) flag(name string) string { return p.flags[name] }
func (p parsedArgs) has(name string) bool    { return p.bools[name] }
func (p parsedArgs) arg(i int) string {
	if i < len(p.positionals) {
		return p.positionals[i]
	}
	return ""
}

// parseArgs tokenises args into flags and positionals. Value flags accept both
// `--flag value` and `--flag=value` forms.
func parseArgs(args []string) (parsedArgs, error) {
	out := parsedArgs{flags: map[string]string{}, bools: map[string]bool{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") || a == "-" {
			out.positionals = append(out.positionals, a)
			continue
		}
		name := strings.TrimLeft(a, "-")
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			out.flags[name[:eq]] = name[eq+1:]
			continue
		}
		if valueFlags[name] {
			if i+1 >= len(args) {
				return out, fmt.Errorf("flag --%s requires a value", name)
			}
			i++
			out.flags[name] = args[i]
			continue
		}
		out.bools[name] = true
	}
	return out, nil
}
