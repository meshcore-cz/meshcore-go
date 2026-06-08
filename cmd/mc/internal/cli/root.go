package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	localbackend "github.com/meshcore-cz/meshcore-go/backend"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/config"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/output"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// App carries per-execution options for a freshly-built Cobra tree.
type App struct {
	Options ExecuteOptions
}

// NewRoot returns the Cobra command tree used by both normal CLI execution and
// the interactive shell. Build a fresh tree for every command execution so flag
// state never leaks between shell commands.
func NewRoot(app *App) *cobra.Command {
	if app == nil {
		app = &App{}
	}
	root := &cobra.Command{
		Use:              "mc",
		Short:            "MeshCore companion radio client",
		SilenceErrors:    true,
		SilenceUsage:     true,
		TraverseChildren: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = cmd.Help()
			return fmt.Errorf("missing command")
		},
	}
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	addGlobalFlags(root)

	root.AddCommand(
		newConnectCommand(app),
		newStatusCommand(app),
		newStatsCommand(app),
		newDoctorCommand(app),
		newBackendCommand(app),
		newContactsCommand(app),
		newContactCommand(app),
		newInboxCommand(app),
		newSendCommand(app),
		newWatchCommand(app),
		newShellCommand(app),
		newTraceCommand(app),
		newChannelCommand(app),
		newAdvertCommand(app),
		newDiscoverCommand(app),
		newRepeaterCommand(app),
		newUseCommand(app),
		newDeviceCommand(app),
		newSessionCommand(app),
		newStateCommand(app),
		newConfigCommand(app),
		newRawCommand(app),
		newVersionCommand(app),
	)
	return root
}

func addGlobalFlags(cmd *cobra.Command) {
	flags := cmd.PersistentFlags()
	flags.Bool("json", false, "machine-readable JSON output")
	flags.String("uri", "", "use an explicit endpoint")
	flags.String("device", "", "use a saved device profile")
	flags.Bool("direct", false, "do not use the local backend")
	flags.Bool("debug", false, "enable verbose logging")
}

func normalizeArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}
	out := append([]string(nil), args...)
	switch out[0] {
	case "?", "h":
		out[0] = "help"
	case "ls", "list":
		out = append([]string{"device", "list"}, out[1:]...)
	case "log":
		out = append([]string{"backend", "log"}, out[1:]...)
	}
	return out
}

type commandHandler func(context.Context, *env) error

func runWithEnv(app *App, restPrefix []string, fn commandHandler) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		e := envFromCommand(app, cmd, appendRest(restPrefix, args))
		return fn(cmd.Context(), e)
	}
}

func runWithEnvNoContext(app *App, restPrefix []string, fn func(*env) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		e := envFromCommand(app, cmd, appendRest(restPrefix, args))
		return fn(e)
	}
}

func appendRest(prefix, args []string) []string {
	rest := append([]string(nil), prefix...)
	return append(rest, args...)
}

func envFromCommand(app *App, cmd *cobra.Command, rest []string) *env {
	pa := parsedArgs{
		flags:       map[string]string{},
		bools:       map[string]bool{},
		positionals: append([]string{cmd.Root().CommandPath()}, rest...),
	}
	collectFlagValues(pa, cmd.InheritedFlags())
	collectFlagValues(pa, cmd.Flags())
	return &env{
		args: pa,
		rest: append([]string(nil), rest...),
		out:  output.New(pa.has("json")),
		dbg:  newDebug(pa),
		exec: app.Options,
	}
}

func collectFlagValues(pa parsedArgs, flags *pflag.FlagSet) {
	if flags == nil {
		return
	}
	flags.VisitAll(func(f *pflag.Flag) {
		switch f.Value.Type() {
		case "bool":
			if f.Value.String() == "true" {
				pa.bools[f.Name] = true
				if f.Shorthand != "" {
					pa.bools[f.Shorthand] = true
				}
			}
		default:
			value := f.Value.String()
			if value != "" {
				pa.flags[f.Name] = value
				if f.Shorthand != "" {
					pa.flags[f.Shorthand] = value
				}
			}
		}
	})
}

func newConnectCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "connect [uri]",
		Aliases: []string{"add"},
		Short:   "Discover or connect to a radio",
		RunE:    runWithEnv(app, nil, cmdConnect),
	}
	cmd.Flags().Bool("usb", false, "scan USB serial only")
	cmd.Flags().Bool("ble", false, "scan BLE only")
	cmd.Flags().String("as", "", "profile name")
	cmd.Flags().Bool("no-save", false, "connect without saving")
	return cmd
}

func newStatusCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "status",
		Aliases: []string{"s"},
		Short:   "Show device status",
		RunE:    runWithEnv(app, nil, cmdStatus),
	}
	cmd.Flags().Bool("all", false, "show compact status for all saved devices")
	cmd.Flags().Bool("live", false, "refresh radio stats before showing status")
	return cmd
}

func newStatsCommand(app *App) *cobra.Command {
	return &cobra.Command{Use: "stats", Short: "Show local radio statistics", RunE: runWithEnv(app, nil, cmdStats)}
}

func newDoctorCommand(app *App) *cobra.Command {
	return &cobra.Command{Use: "doctor", Short: "Run connection diagnostics", RunE: runWithEnv(app, nil, cmdDoctor)}
}

func newContactsCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "contacts",
		Aliases: []string{"c"},
		Short:   "List contacts",
		RunE:    runWithEnv(app, nil, cmdContacts),
	}
	cmd.Flags().Bool("wide", false, "show paths and coordinates")
	cmd.Flags().Bool("refresh", false, "refresh contact local state")
	cmd.Flags().Bool("wait", false, "wait for refresh to finish")
	cmd.Flags().Bool("full", false, "perform a full contact sync")
	cmd.Flags().Bool("cached", false, "read device-local state only")
	cmd.Flags().String("type", "", "filter by contact type")
	cmd.Flags().String("route", "", "filter by route kind")
	cmd.Flags().String("within", "", "filter by distance")
	cmd.Flags().String("sort", "", "sort field")
	return cmd
}

func newContactCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "contact", Short: "Show a contact", RunE: runWithEnv(app, nil, cmdContact)}
	cmd.AddCommand(&cobra.Command{Use: "show <name>", Short: "Show contact details", Args: cobra.ExactArgs(1), ValidArgsFunction: completeContactsCobra, RunE: runWithEnv(app, []string{"show"}, cmdContact)})
	return cmd
}

func newInboxCommand(app *App) *cobra.Command {
	return &cobra.Command{Use: "inbox", Short: "Drain buffered incoming messages", RunE: runWithEnv(app, nil, cmdInbox)}
}

func newSendCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "send <recipient> <text>", Short: "Send a direct or channel message", ValidArgsFunction: completeContactsCobra, RunE: runWithEnv(app, nil, cmdSend)}
	cmd.Flags().Bool("wait", false, "wait for acknowledgement")
	cmd.Flags().String("channel", "", "send to a channel")
	return cmd
}

func newWatchCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "watch", Short: "Stream incoming messages and events", RunE: runWithEnv(app, nil, cmdWatch)}
	cmd.Flags().Bool("rf", false, "stream RF packet log frames as JSON")
	cmd.Flags().Bool("raw", false, "stream raw packets as JSON")
	return cmd
}

func newShellCommand(app *App) *cobra.Command {
	return &cobra.Command{Use: "shell", Short: "Open an interactive persistent session", RunE: runWithEnv(app, nil, func(ctx context.Context, e *env) error {
		if e.exec.InShell {
			return fmt.Errorf("already running inside mc shell")
		}
		return cmdShell(ctx, e)
	})}
}

func newTraceCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "trace <target>", Short: "Trace route to a contact or path", ValidArgsFunction: completeContactsCobra, RunE: runWithEnv(app, nil, cmdTrace)}
	cmd.Flags().Bool("return", false, "append reverse path for return tracing")
	return cmd
}

func newChannelCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "channel", Short: "List or use channels", RunE: runWithEnv(app, nil, cmdChannel)}
	cmd.Flags().Bool("refresh", false, "refresh channel local state")
	list := &cobra.Command{Use: "list", Short: "List channels", RunE: runWithEnv(app, []string{"list"}, cmdChannel)}
	list.Flags().Bool("refresh", false, "refresh channel local state")
	cmd.AddCommand(
		list,
		&cobra.Command{Use: "show <name|index>", Short: "Show channel details", Args: cobra.ExactArgs(1), ValidArgsFunction: completeChannelsCobra, RunE: runWithEnv(app, []string{"show"}, cmdChannel)},
		&cobra.Command{Use: "send <name|index> <text>", Short: "Send a channel message", Args: cobra.ExactArgs(2), ValidArgsFunction: completeChannelsCobra, RunE: runWithEnv(app, []string{"send"}, cmdChannel)},
		&cobra.Command{Use: "add <name> [key]", Short: "Add a channel", Args: cobra.RangeArgs(1, 2), RunE: runWithEnv(app, []string{"add"}, cmdChannel)},
		&cobra.Command{Use: "remove <name|index>", Aliases: []string{"rm", "del"}, Short: "Remove a channel", Args: cobra.ExactArgs(1), ValidArgsFunction: completeChannelsCobra, RunE: runWithEnv(app, []string{"remove"}, cmdChannel)},
	)
	return cmd
}

func newAdvertCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "advert", Short: "Broadcast this device's advert", RunE: runWithEnv(app, nil, cmdAdvert)}
	cmd.Flags().Bool("flood", false, "flood advert mesh-wide")
	return cmd
}

func newDiscoverCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "discover", Short: "Scan for nearby nodes", RunE: runWithEnv(app, nil, cmdDiscover)}
	cmd.Flags().Bool("all", false, "discover every node type")
	cmd.Flags().Bool("repeater", false, "discover repeaters")
	cmd.Flags().Bool("companion", false, "discover companion nodes")
	cmd.Flags().Bool("client", false, "discover companion nodes")
	cmd.Flags().Bool("room", false, "discover room servers")
	cmd.Flags().Bool("sensor", false, "discover sensors")
	cmd.Flags().Bool("full", false, "request full public keys")
	cmd.Flags().String("timeout", "", "listen duration in seconds")
	return cmd
}

func newBackendCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "backend", Aliases: []string{"b"}, Short: "Manage the local backend", RunE: runWithEnv(app, nil, cmdBackend)}
	cmd.Flags().Bool("verbose", false, "verbose backend status")
	status := &cobra.Command{Use: "status", Short: "Show backend diagnostics", RunE: runWithEnv(app, []string{"status"}, cmdBackend)}
	status.Flags().Bool("verbose", false, "verbose backend status")
	restart := &cobra.Command{Use: "restart", Short: "Restart backend", RunE: runWithEnv(app, []string{"restart"}, cmdBackend)}
	restart.Flags().Bool("reset", false, "reset local state")
	logCmd := &cobra.Command{Use: "log", Aliases: []string{"logs"}, Short: "Show backend logs", RunE: runWithEnv(app, []string{"log"}, cmdBackend)}
	logCmd.Flags().BoolP("follow", "f", false, "follow log output")
	logCmd.Flags().StringP("lines", "n", "", "number of lines")
	cmd.AddCommand(
		status,
		&cobra.Command{Use: "start", Short: "Start backend", RunE: runWithEnv(app, []string{"start"}, cmdBackend)},
		&cobra.Command{Use: "stop", Short: "Stop backend", RunE: runWithEnv(app, []string{"stop"}, cmdBackend)},
		restart,
		&cobra.Command{Use: "reset", Short: "Stop backend and delete local state", RunE: runWithEnv(app, []string{"reset"}, cmdBackend)},
		&cobra.Command{Use: "serve", Short: "Run backend in foreground", RunE: runWithEnv(app, []string{"serve"}, cmdBackend)},
		logCmd,
	)
	return cmd
}

func newRepeaterCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "repeater", Aliases: []string{"rep"}, Short: "Manage saved repeaters", RunE: runWithEnv(app, nil, cmdRepeater)}
	cmd.AddCommand(
		&cobra.Command{Use: "list", Short: "List saved repeaters", RunE: runWithEnv(app, []string{"list"}, cmdRepeater)},
		&cobra.Command{Use: "add <name> [password]", Short: "Save/login to a repeater", Args: cobra.RangeArgs(1, 2), ValidArgsFunction: completeRepeatersCobra, RunE: runWithEnv(app, []string{"add"}, cmdRepeater)},
		&cobra.Command{Use: "del [name]", Aliases: []string{"delete", "remove"}, Short: "Remove a saved repeater", Args: cobra.MaximumNArgs(1), ValidArgsFunction: completeRepeatersCobra, RunE: runWithEnv(app, []string{"del"}, cmdRepeater)},
		&cobra.Command{Use: "status [name]", Short: "Query repeater status", Args: cobra.MaximumNArgs(1), ValidArgsFunction: completeRepeatersCobra, RunE: runWithEnv(app, []string{"status"}, cmdRepeater)},
		&cobra.Command{Use: "neighbours [name]", Aliases: []string{"neighbors"}, Short: "List repeater neighbours", Args: cobra.MaximumNArgs(1), ValidArgsFunction: completeRepeatersCobra, RunE: runWithEnv(app, []string{"neighbours"}, cmdRepeater)},
		&cobra.Command{Use: "exec [name] <command>", Short: "Run repeater command", Args: cobra.ArbitraryArgs, ValidArgsFunction: completeRepeatersCobra, RunE: runWithEnv(app, []string{"exec"}, cmdRepeater)},
	)
	return cmd
}

func newUseCommand(app *App) *cobra.Command {
	return &cobra.Command{Use: "use <profile>", Short: "Set the default device profile", Args: cobra.ExactArgs(1), ValidArgsFunction: completeDeviceProfilesCobra, RunE: runWithEnvNoContext(app, nil, cmdUse)}
}

func newDeviceCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "device", Aliases: []string{"d", "devices"}, Short: "Manage saved device profiles", RunE: runWithEnv(app, nil, cmdDevice)}
	cmd.Flags().Bool("wide", false, "wide output")
	list := &cobra.Command{Use: "list", Short: "List saved profiles", RunE: runWithEnv(app, []string{"list"}, cmdDevice)}
	list.Flags().Bool("wide", false, "wide output")
	cmd.AddCommand(
		list,
		&cobra.Command{Use: "show [name]", Short: "Show a profile", Args: cobra.MaximumNArgs(1), ValidArgsFunction: completeDeviceProfilesCobra, RunE: runWithEnv(app, []string{"show"}, cmdDevice)},
		&cobra.Command{Use: "remove <name>", Short: "Remove a profile", Args: cobra.ExactArgs(1), ValidArgsFunction: completeDeviceProfilesCobra, RunE: runWithEnv(app, []string{"remove"}, cmdDevice)},
	)
	return cmd
}

func newSessionCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "session", Short: "Manage device sessions", RunE: runWithEnv(app, nil, cmdSession)}
	cmd.AddCommand(
		&cobra.Command{Use: "list", Short: "Show device session states", RunE: runWithEnv(app, []string{"list"}, cmdSession)},
		&cobra.Command{Use: "start [name]", Short: "Connect a device session", Args: cobra.MaximumNArgs(1), ValidArgsFunction: completeDeviceProfilesCobra, RunE: runWithEnv(app, []string{"start"}, cmdSession)},
		&cobra.Command{Use: "stop [name]", Short: "Disconnect a device session", Args: cobra.MaximumNArgs(1), ValidArgsFunction: completeDeviceProfilesCobra, RunE: runWithEnv(app, []string{"stop"}, cmdSession)},
		&cobra.Command{Use: "restart [name]", Short: "Reconnect a device session", Args: cobra.MaximumNArgs(1), ValidArgsFunction: completeDeviceProfilesCobra, RunE: runWithEnv(app, []string{"restart"}, cmdSession)},
	)
	return cmd
}

func newStateCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "state", Short: "Inspect per-device local state", RunE: runWithEnvNoContext(app, nil, cmdState)}
	cmd.AddCommand(
		&cobra.Command{Use: "list", Short: "List per-device local-state databases", RunE: runWithEnvNoContext(app, []string{"list"}, cmdState)},
		&cobra.Command{Use: "show <device>", Short: "Show one device's local state", Args: cobra.ExactArgs(1), ValidArgsFunction: completeDeviceProfilesCobra, RunE: runWithEnvNoContext(app, []string{"show"}, cmdState)},
		&cobra.Command{Use: "purge <device>", Short: "Delete one device's local state", Args: cobra.ExactArgs(1), ValidArgsFunction: completeDeviceProfilesCobra, RunE: runWithEnvNoContext(app, []string{"purge"}, cmdState)},
	)
	prune := &cobra.Command{Use: "prune", Short: "Delete state older than a duration", RunE: runWithEnvNoContext(app, []string{"prune"}, cmdState)}
	prune.Flags().String("older-than", "", "age threshold")
	cmd.AddCommand(prune)
	return cmd
}

func newConfigCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "config", Aliases: []string{"conf"}, Short: "Show configuration", RunE: runWithEnvNoContext(app, nil, cmdConfig)}
	cmd.AddCommand(
		&cobra.Command{Use: "path", Short: "Print config file path", RunE: runWithEnvNoContext(app, []string{"path"}, cmdConfig)},
		&cobra.Command{Use: "show", Short: "Print current configuration", RunE: runWithEnvNoContext(app, []string{"show"}, cmdConfig)},
	)
	return cmd
}

func newRawCommand(app *App) *cobra.Command {
	return &cobra.Command{Use: "raw <hex>", Short: "Send raw bytes to the radio", Args: cobra.ArbitraryArgs, RunE: runWithEnv(app, nil, cmdRaw)}
}

func newVersionCommand(app *App) *cobra.Command {
	return &cobra.Command{Use: "version", Short: "Show version information", RunE: runWithEnvNoContext(app, nil, cmdVersion)}
}

func completeContactsCobra(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	client := localbackend.NewClient("")
	contacts, err := client.ContactsWithOptions(cmd.Context(), true, false, false, false)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	items := make([]string, 0, len(contacts))
	for _, ct := range contacts {
		if ct.Name == "" {
			continue
		}
		items = append(items, cobraCompletionValue(ct.Name, humanContactType(ct.Type)))
	}
	return filterCobraCompletions(items, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func completeChannelsCobra(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	client := localbackend.NewClient("")
	channels, err := client.ChannelsWithOptions(cmd.Context(), false)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	items := make([]string, 0, len(channels))
	for _, ch := range channels {
		name := ch.Name
		if name == "" {
			name = strconv.Itoa(int(ch.Index))
		}
		items = append(items, cobraCompletionValue(name, "channel"))
	}
	return filterCobraCompletions(items, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func completeDeviceProfilesCobra(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	items := make([]string, 0, len(cfg.Devices))
	for name := range cfg.Devices {
		desc := "device profile"
		if cfg.Current == name {
			desc = "current profile"
		}
		items = append(items, cobraCompletionValue(name, desc))
	}
	return filterCobraCompletions(items, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func completeRepeatersCobra(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	items := make([]string, 0, len(cfg.Repeaters))
	for key, rep := range cfg.Repeaters {
		name := rep.Name
		if name == "" {
			name = key
		}
		items = append(items, cobraCompletionValue(name, "repeater profile"))
	}
	return filterCobraCompletions(items, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func cobraCompletionValue(value, description string) string {
	if description == "" {
		return value
	}
	return value + "\t" + description
}

func filterCobraCompletions(items []string, prefix string) []string {
	if prefix == "" {
		return items
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		value, _, _ := strings.Cut(item, "\t")
		if stringsHasPrefixFold(value, prefix) {
			out = append(out, item)
		}
	}
	return out
}

// writeCommandHelp writes Cobra-generated help for a command to stdout.
func writeCommandHelp(name string) error {
	root := NewRoot(&App{})
	found, _, err := root.Find(normalizeArgs([]string{name}))
	if err != nil || found == root {
		fmt.Fprintf(os.Stderr, "mc: no help available for %q\n", name)
		_ = root.Help()
		return fmt.Errorf("no help available for %q", name)
	}
	return found.Help()
}

// printCommandHelp writes a command's help to stdout and returns an exit code.
func printCommandHelp(name string) int {
	if err := writeCommandHelp(name); err != nil {
		return 2
	}
	return 0
}
