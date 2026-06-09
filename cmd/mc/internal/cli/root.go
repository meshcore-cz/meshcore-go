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
	root.SetHelpTemplate(helpTemplate)
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
		newChatCommand(app),
		newWatchCommand(app),
		newShellCommand(app),
		newTraceCommand(app),
		newChannelCommand(app),
		newAdvertCommand(app),
		newShareCommand(app),
		newDiscoverCommand(app),
		newRepeaterCommand(app),
		newUseCommand(app),
		newDeviceCommand(app),
		newSessionCommand(app),
		newStateCommand(app),
		newConfigCommand(app),
		newCompletionCommand(app),
		newRawCommand(app),
		newPktCommand(app),
		newVersionCommand(app),
	)
	return root
}

const helpTemplate = `{{with .Short}}{{.}}

{{end}}Usage:
  {{.UseLine}}{{if .HasAvailableSubCommands}}

Available Commands:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if and .Long (ne .Long .Short)}}

Description:
{{.Long}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}
`

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
		Long: strings.TrimSpace(`
Discover or connect to a companion radio, verify it with a handshake, and
(unless --no-save) save a profile and make it the default. In interactive mode,
mc then offers to start the local backend for that endpoint.`),
		Example: strings.TrimSpace(`
mc connect
mc connect --usb
mc connect serial:///dev/ttyACM0
mc connect ble://C4:20:... --as handheld`),
		RunE: runWithEnv(app, nil, cmdConnect),
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
		Long:    "Show radio status, persistent local state, and backend session state.",
		Example: strings.TrimSpace(`
mc status
mc status --live
mc status --all`),
		RunE: runWithEnv(app, nil, cmdStatus),
	}
	cmd.Flags().Bool("all", false, "show compact status for all saved devices")
	cmd.Flags().Bool("live", false, "refresh radio stats before showing status")
	return cmd
}

func newStatsCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "stats",
		Short:   "Show local radio statistics",
		Long:    "Show local core, radio and packet statistics from the active companion radio.",
		Example: "mc stats\nmc stats --live",
		RunE:    runWithEnv(app, nil, cmdStats),
	}
}

func newDoctorCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "doctor",
		Short:   "Run connection diagnostics",
		Long:    "Run connection diagnostics: configuration, endpoint reachability, handshake, firmware and clock difference.",
		Example: "mc doctor\nmc doctor --direct",
		RunE:    runWithEnv(app, nil, cmdDoctor),
	}
}

func newContactsCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "contacts",
		Aliases: []string{"c"},
		Short:   "List contacts",
		Long: strings.TrimSpace(`
List contacts from the backend's device-local state.

Use --refresh to start a radio sync. With --wait, mc blocks until synchronization
finishes; otherwise the sync runs in the background.`),
		Example: strings.TrimSpace(`
mc contacts
mc contacts --refresh --wait
mc contacts --type repeater --sort age
mc contacts --within 10km --wide`),
		RunE: runWithEnv(app, nil, cmdContacts),
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
	cmd := &cobra.Command{
		Use:     "contact",
		Short:   "Show a contact",
		Long:    "Show details for a single contact, matched by name (case-insensitive) or by a public-key hex prefix.",
		Example: "mc contact show alice\nmc contact show eff01ef2",
		RunE:    runWithEnv(app, nil, cmdContact),
	}
	cmd.AddCommand(&cobra.Command{Use: "show <name>", Short: "Show contact details", Example: "mc contact show alice", Args: cobra.ExactArgs(1), ValidArgsFunction: completeContactsCobra, RunE: runWithEnv(app, []string{"show"}, cmdContact)})
	return cmd
}

func newInboxCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "inbox",
		Short: "Drain buffered incoming messages",
		Long: strings.TrimSpace(`
Print unread incoming messages and mark them read. When the backend is running,
it drains the radio inbox itself, persists every message to device-local state,
and broadcasts it to mc watch; mc inbox then reads the stored unread messages.
Without a backend, mc inbox drains the radio directly.`),
		Example: "mc inbox\nmc inbox --json",
		RunE:    runWithEnv(app, nil, cmdInbox),
	}
}

func newSendCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send <recipient> <text>",
		Short: "Send a direct or channel message",
		Long: strings.TrimSpace(`
Send a direct text message to a contact (by name or key prefix), or a message to
a channel with --channel.`),
		Example: strings.TrimSpace(`
mc send alice "hello"
mc send alice "hello" --wait
mc send --channel rem-ha "ahoj!"`),
		ValidArgsFunction: completeContactsCobra,
		RunE:              runWithEnv(app, nil, cmdSend),
	}
	cmd.Flags().Bool("wait", false, "wait for acknowledgement")
	cmd.Flags().String("channel", "", "send to a channel")
	return cmd
}

func newChatCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "chat <contact|channel>",
		Short: "Open an interactive chat",
		Long: strings.TrimSpace(`
Open a full-screen interactive chat with a contact or channel.

The target is matched against contacts first (by name or key prefix), then
channels (by name or index). When the local backend is running, prior messages
are loaded from device-local state and new messages stream in live; otherwise mc
connects directly to the radio and shows live messages only.

Press Enter to send, PgUp/PgDn to scroll, and Ctrl-C to quit.`),
		Example:           "mc chat alice\nmc chat Public\nmc chat #test",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeContactsCobra,
		RunE:              runWithEnv(app, nil, cmdChat),
	}
}

func newWatchCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "watch",
		Short:   "Stream incoming messages and events",
		Long:    "Stream incoming messages and events until interrupted (Ctrl-C). With --json, each event is emitted as a newline-delimited JSON object.",
		Example: "mc watch\nmc watch --raw --json\nmc watch --rf --json",
		RunE:    runWithEnv(app, nil, cmdWatch),
	}
	cmd.Flags().Bool("rf", false, "stream RF packet log frames as JSON")
	cmd.Flags().Bool("raw", false, "stream raw packets as JSON")
	return cmd
}

func newShellCommand(app *App) *cobra.Command {
	return &cobra.Command{Use: "shell", Short: "Open an interactive persistent session", Long: "Open an interactive foreground session that keeps one radio connection alive.", Example: "mc shell\nmc shell --device handheld", RunE: runWithEnv(app, nil, func(ctx context.Context, e *env) error {
		if e.exec.InShell {
			return fmt.Errorf("already running inside mc shell")
		}
		return cmdShell(ctx, e)
	})}
}

func newTraceCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trace <target>",
		Short: "Trace route to a contact or path",
		Long: strings.TrimSpace(`
Trace the network route to a node and report per-leg signal quality.

The target is either a hash path or a contact name. Hex targets are always
interpreted as explicit paths. Use a contact name to trace using a stored contact
route.`),
		Example: strings.TrimSpace(`
mc trace alice
mc trace 25
mc trace 25,a1
mc trace 2525,5153,0455 --return`),
		ValidArgsFunction: completeContactsCobra,
		RunE:              runWithEnv(app, nil, cmdTrace),
	}
	cmd.Flags().Bool("return", false, "append reverse path for return tracing")
	return cmd
}

func newChannelCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "channel",
		Short: "List or use channels",
		Long: strings.TrimSpace(`
Work with channel slots.

add/remove write to the device first, then re-sync local state from the device.
A #name channel derives its key from the name; otherwise a 16-byte key (hex or
base64) is required.

Channels are served from the backend's local device state. Use --refresh to
force a radio sync.`),
		Example: strings.TrimSpace(`
mc channel list
mc channel show Public
mc channel send Public "hello"
mc channel add rem-ha <key>
mc channel add #test
mc channel remove rem-ha`),
		RunE: runWithEnv(app, nil, cmdChannel),
	}
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
	cmd := &cobra.Command{
		Use:     "advert",
		Short:   "Broadcast this device's advert",
		Long:    "Broadcast the device's own advertisement so other nodes learn about it.",
		Example: "mc advert\nmc advert --flood",
		RunE:    runWithEnv(app, nil, cmdAdvert),
	}
	cmd.Flags().Bool("flood", false, "flood advert mesh-wide")
	return cmd
}

func newShareCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "share",
		Aliases: []string{"qr"},
		Short:   "Show a QR code to add this device as a contact",
		Long: strings.TrimSpace(`
Print a QR code and meshcore:// link for the connected device so others can scan
it to add the device as a contact in the MeshCore app.`),
		Example: strings.TrimSpace(`
mc share
mc share --no-qr
mc share --json`),
		RunE: runWithEnv(app, nil, cmdShare),
	}
	cmd.Flags().Bool("no-qr", false, "print the link only, without the QR code")
	cmd.Flags().String("type", "", "advert type: 1=companion 2=repeater 3=room 4=sensor (default 1)")
	return cmd
}

func newDiscoverCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Scan for nearby nodes",
		Long: strings.TrimSpace(`
Broadcast a node-discovery request and print nodes as they reply. Each reply
reports the round-trip signal: up is how the remote node heard the request, down
is how this device heard the reply. With no type flag, only repeaters are
scanned.`),
		Example: strings.TrimSpace(`
mc discover
mc discover --all
mc discover --room --sensor
mc discover --timeout 10`),
		RunE: runWithEnv(app, nil, cmdDiscover),
	}
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
	cmd := &cobra.Command{
		Use:     "backend",
		Aliases: []string{"b"},
		Short:   "Manage the local backend",
		Long: strings.TrimSpace(`
Manage the local backend process. When it is running, ordinary commands use it
automatically; when it is not running, commands dial the radio directly.

Bridge listeners are configured in config.yaml:

backend:
  log_requests: true
  bridges:
    - enabled: true
      type: tcp
      listen: 127.0.0.1:4403
    - enabled: true
      type: pty

Note: on macOS, Chrome/Web Serial does not list PTY bridges. The pty bridge is
for native serial clients; use tcp only with clients that explicitly support a
TCP MeshCore bridge.`),
		Example: strings.TrimSpace(`
mc backend start
mc backend status --verbose
mc backend log --follow
mc backend restart --reset
mc backend stop`),
		RunE: runWithEnv(app, nil, cmdBackend),
	}
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
	cmd := &cobra.Command{
		Use:     "repeater",
		Aliases: []string{"rep"},
		Short:   "Manage saved repeaters",
		Long: strings.TrimSpace(`
Manage remote repeaters through the active companion radio. Saved repeaters are
stored in the mc config. If no password is supplied to add, mc prompts for one
in interactive human mode.`),
		Example: strings.TrimSpace(`
mc repeater list
mc repeater add mc.kololec.cz
mc repeater del mc.kololec.cz
mc repeater status mc.kololec.cz
mc repeater neighbours mc.kololec.cz
mc repeater exec mc.kololec.cz "clock"`),
		RunE: runWithEnv(app, nil, cmdRepeater),
	}
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
	return &cobra.Command{Use: "use <profile>", Short: "Set the default device profile", Example: "mc use handheld", Args: cobra.ExactArgs(1), ValidArgsFunction: completeDeviceProfilesCobra, RunE: runWithEnvNoContext(app, nil, cmdUse)}
}

func newDeviceCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "device",
		Aliases: []string{"d", "devices"},
		Short:   "Manage saved device profiles",
		Long: strings.TrimSpace(`
Manage saved device profiles. To connect or disconnect a device's live radio
session, use mc session.`),
		Example: strings.TrimSpace(`
mc device list
mc device list --wide
mc device show
mc device show handheld
mc device remove handheld`),
		RunE: runWithEnv(app, nil, cmdDevice),
	}
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
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Manage device sessions",
		Long: strings.TrimSpace(`
Manage device sessions: the live radio connections held by the running backend.
A session connects one saved profile's radio; profiles are managed with mc
device. These require a running backend (mc backend start).

With no name, the current default device is used. Each session is isolated: the
radio connection, queue, and local state sync for other devices are unaffected.
Set backend.autostart: true on a profile to connect it automatically when the
backend starts.`),
		Example: strings.TrimSpace(`
mc session list
mc session start handheld
mc session stop handheld
mc session restart handheld`),
		RunE: runWithEnv(app, nil, cmdSession),
	}
	cmd.AddCommand(
		&cobra.Command{Use: "list", Short: "Show device session states", RunE: runWithEnv(app, []string{"list"}, cmdSession)},
		&cobra.Command{Use: "start [name]", Short: "Connect a device session", Args: cobra.MaximumNArgs(1), ValidArgsFunction: completeDeviceProfilesCobra, RunE: runWithEnv(app, []string{"start"}, cmdSession)},
		&cobra.Command{Use: "stop [name]", Short: "Disconnect a device session", Args: cobra.MaximumNArgs(1), ValidArgsFunction: completeDeviceProfilesCobra, RunE: runWithEnv(app, []string{"stop"}, cmdSession)},
		&cobra.Command{Use: "restart [name]", Short: "Reconnect a device session", Args: cobra.MaximumNArgs(1), ValidArgsFunction: completeDeviceProfilesCobra, RunE: runWithEnv(app, []string{"restart"}, cmdSession)},
	)
	return cmd
}

func newStateCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "state",
		Short: "Inspect per-device local state",
		Long: strings.TrimSpace(`
Inspect and manage per-device local state. Each connected device keeps its own
SQLite database of contacts, channels, and repeater sessions at
~/.local/state/mc/devices/<public-key-prefix>.db. This is device-local state,
not a cache: it may be stale, incomplete, or locally enriched.

A device may be named by saved profile, full public key, or key prefix.
Durations accept d (days), w (weeks), and Go units (h, m, s).`),
		Example: strings.TrimSpace(`
mc state list
mc state show handheld
mc state purge handheld
mc state prune --older-than 30d`),
		RunE: runWithEnvNoContext(app, nil, cmdState),
	}
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
	cmd := &cobra.Command{
		Use:     "config",
		Aliases: []string{"conf"},
		Short:   "Show configuration",
		Long:    "Inspect the CLI configuration.",
		Example: "mc config path\nmc config show",
		RunE:    runWithEnvNoContext(app, nil, cmdConfig),
	}
	cmd.AddCommand(
		&cobra.Command{Use: "path", Short: "Print config file path", RunE: runWithEnvNoContext(app, []string{"path"}, cmdConfig)},
		&cobra.Command{Use: "show", Short: "Print current configuration", RunE: runWithEnvNoContext(app, []string{"show"}, cmdConfig)},
	)
	return cmd
}

func newCompletionCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion",
		Short: "Generate shell completion scripts",
		Long: strings.TrimSpace(`
Generate shell completion scripts for mc.

For one-shot use, source the generated script in the current shell. For
permanent installation, write it to your shell's completion directory.`),
		Example: strings.TrimSpace(`
mc completion bash
mc completion zsh
mc completion fish | source
mkdir -p ~/.local/share/bash-completion/completions
mc completion bash > ~/.local/share/bash-completion/completions/mc
mkdir -p ~/.zsh/completions
mc completion zsh > ~/.zsh/completions/_mc
mkdir -p ~/.config/fish/completions
mc completion fish > ~/.config/fish/completions/mc.fish`),
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "bash",
		Short: "Generate bash completion script",
		Long: strings.TrimSpace(`
Generate a bash completion script for mc.

To load completion in the current shell:

  source <(mc completion bash)

To install completion permanently:

  mkdir -p ~/.local/share/bash-completion/completions
  mc completion bash > ~/.local/share/bash-completion/completions/mc`),
		Example: strings.TrimSpace(`
source <(mc completion bash)
mc completion bash > ~/.local/share/bash-completion/completions/mc`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Root().GenBashCompletion(cmd.OutOrStdout())
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "zsh",
		Short: "Generate zsh completion script",
		Long: strings.TrimSpace(`
Generate a zsh completion script for mc.

To load completion in the current shell:

  source <(mc completion zsh)

To install completion permanently:

  mkdir -p ~/.zsh/completions
  mc completion zsh > ~/.zsh/completions/_mc

Make sure ~/.zsh/completions is in fpath before compinit runs.`),
		Example: strings.TrimSpace(`
source <(mc completion zsh)
mc completion zsh > ~/.zsh/completions/_mc`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Root().GenZshCompletion(cmd.OutOrStdout())
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "fish",
		Short: "Generate fish completion script",
		Long: strings.TrimSpace(`
Generate a fish completion script for mc.

To load completion in the current shell:

  mc completion fish | source

To install completion permanently:

  mkdir -p ~/.config/fish/completions
  mc completion fish > ~/.config/fish/completions/mc.fish`),
		Example: strings.TrimSpace(`
mc completion fish | source
mc completion fish > ~/.config/fish/completions/mc.fish`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Root().GenFishCompletion(cmd.OutOrStdout(), true)
		},
	})
	return cmd
}

func newRawCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "raw <hex>",
		Short: "Send raw bytes directly to the device and print the decoded response",
		Long: strings.TrimSpace(`
Send raw bytes directly to the device and print the decoded response.
Useful for protocol exploration and verifying undocumented commands.

The payload is the bare companion-protocol bytes (command byte + body).
Transport framing is added automatically; do not include it.
Bytes can be given as separate tokens or concatenated.

Common command bytes (host -> device):
  0x01  AppStart          0x0a  SyncNextMessage
  0x02  SendTxtMsg        0x0b  SetRadioParams
  0x03  SendChannelTxt    0x0c  SetTxPower
  0x04  GetContacts       0x0d  ResetPath
  0x05  GetDeviceTime     0x13  Reboot
  0x06  SetDeviceTime     0x14  GetBatteryVoltage
  0x07  SendSelfAdvert    0x16  DeviceQuery
  0x09  AddUpdateContact  0x1f  GetChannel
  0x0f  RemoveContact     0x24  SendTracePath
  0x38  GetStats

Output shows the decoded response type and fields for known opcodes, or a hex
dump for unrecognised ones. With --debug, logs the resolved endpoint, outbound
frame, and decoded response.`),
		Example: strings.TrimSpace(`
mc raw 16 03
mc raw 14
mc raw 0xab 0xcd
mc raw abcdef`),
		Args: cobra.ArbitraryArgs,
		RunE: runWithEnv(app, nil, cmdRaw),
	}
}

func newVersionCommand(app *App) *cobra.Command {
	return &cobra.Command{Use: "version", Short: "Show version information", Long: "Print version information.", Example: "mc version\nmc version --json", RunE: runWithEnvNoContext(app, nil, cmdVersion)}
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
