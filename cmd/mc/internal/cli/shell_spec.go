package cli

import "context"

var globalShellFlags = []FlagSpec{
	{Name: "--json", Description: "Machine-readable JSON output"},
	{Name: "--uri", Description: "Use an explicit endpoint", TakesValue: true},
	{Name: "--device", Description: "Use a saved profile", TakesValue: true},
	{Name: "--direct", Description: "Do not use the local backend"},
	{Name: "--debug", Description: "Verbose logging"},
}

// CommandSpec describes one mc command for completion and help metadata.
type CommandSpec struct {
	Name         string
	Aliases      []string
	Description  string
	Flags        []FlagSpec
	Children     []CommandSpec
	CompleteArgs CompletionFunc
	Run          CommandFunc
}

type CommandFunc func(context.Context, *env) error

type FlagSpec struct {
	Name        string
	Description string
	TakesValue  bool
}

type CompletionFunc func(ctx context.Context, session *shellSession, args []string, endsWithSpace bool) []CompletionItem

type CompletionItem struct {
	Value       string
	Description string
}

var commandRegistry = []CommandSpec{
	{
		Name:        "status",
		Aliases:     []string{"s"},
		Description: "Show device status",
		Flags: []FlagSpec{
			{Name: "--all", Description: "Show compact status for all saved devices"},
			{Name: "--live", Description: "Refresh radio stats before showing status"},
		},
		Run: cmdStatus,
	},
	{
		Name:        "stats",
		Description: "Show local radio statistics",
		Run:         cmdStats,
	},
	{
		Name:        "doctor",
		Description: "Run connection diagnostics",
		Run:         cmdDoctor,
	},
	{
		Name:        "contacts",
		Aliases:     []string{"c"},
		Description: "List contacts",
		Flags: []FlagSpec{
			{Name: "--wide", Description: "Show paths and coordinates"},
			{Name: "--refresh", Description: "Refresh contact local state"},
			{Name: "--wait", Description: "Wait for refresh to finish"},
			{Name: "--full", Description: "Full contact sync"},
			{Name: "--cached", Description: "Read device-local state only"},
			{Name: "--type", Description: "Filter by contact type", TakesValue: true},
			{Name: "--route", Description: "Filter by route kind", TakesValue: true},
			{Name: "--within", Description: "Filter by distance", TakesValue: true},
			{Name: "--sort", Description: "Sort field", TakesValue: true},
		},
		Run: cmdContacts,
	},
	{
		Name:        "contact",
		Description: "Show a contact",
		Children: []CommandSpec{
			{
				Name:         "show",
				Description:  "Show contact details",
				CompleteArgs: completeContactsArg,
			},
		},
		Run: cmdContact,
	},
	{
		Name:        "inbox",
		Description: "Drain buffered incoming messages",
		Run:         cmdInbox,
	},
	{
		Name:         "send",
		Description:  "Send a direct or channel message",
		CompleteArgs: completeContactsArg,
		Flags: []FlagSpec{
			{Name: "--wait", Description: "Wait for acknowledgement"},
			{Name: "--channel", Description: "Send to a channel", TakesValue: true},
		},
		Run: cmdSend,
	},
	{
		Name:         "trace",
		Description:  "Trace route to a contact or path",
		CompleteArgs: completeContactsArg,
		Flags: []FlagSpec{
			{Name: "--return", Description: "Append the reverse path for return tracing"},
		},
		Run: cmdTrace,
	},
	{
		Name:        "channel",
		Description: "List or use channels",
		Children: []CommandSpec{
			{
				Name:        "list",
				Description: "List channels",
				Flags: []FlagSpec{
					{Name: "--refresh", Description: "Refresh channel local state"},
				},
			},
			{
				Name:         "show",
				Description:  "Show channel details",
				CompleteArgs: completeChannelsArg,
			},
			{
				Name:         "send",
				Description:  "Send a channel message",
				CompleteArgs: completeChannelsArg,
			},
			{Name: "add", Description: "Add a channel (key or #hash)"},
			{Name: "remove", Description: "Remove a channel", CompleteArgs: completeChannelsArg},
		},
		Run: cmdChannel,
	},
	{
		Name:        "advert",
		Description: "Broadcast this device's advert",
		Flags: []FlagSpec{
			{Name: "--flood", Description: "Flood advert mesh-wide"},
		},
		Run: cmdAdvert,
	},
	{
		Name:        "discover",
		Description: "Scan for nearby nodes",
		Run:         cmdDiscover,
	},
	{
		Name:        "watch",
		Description: "Stream incoming messages and events",
		Flags: []FlagSpec{
			{Name: "--rf", Description: "Stream RF packet log frames as JSON"},
			{Name: "--raw", Description: "Stream raw packets as JSON"},
		},
		Run: cmdWatch,
	},
	{
		Name:        "backend",
		Aliases:     []string{"b"},
		Description: "Manage the local backend",
		Children: []CommandSpec{
			{Name: "status", Description: "Show backend diagnostics", Flags: []FlagSpec{{Name: "--verbose", Description: "Verbose backend status"}}},
			{Name: "start", Description: "Start backend"},
			{Name: "stop", Description: "Stop backend"},
			{Name: "restart", Description: "Restart backend", Flags: []FlagSpec{{Name: "--reset", Description: "Reset local state"}}},
			{Name: "reset", Description: "Stop backend and delete local state"},
			{Name: "serve", Description: "Run backend in foreground"},
			{Name: "log", Description: "Show backend logs", Flags: []FlagSpec{
				{Name: "--follow", Description: "Follow log output"},
				{Name: "-f", Description: "Follow log output"},
				{Name: "--lines", Description: "Number of lines", TakesValue: true},
				{Name: "-n", Description: "Number of lines", TakesValue: true},
			}},
		},
		Run: cmdBackend,
	},
	{
		Name:         "repeater",
		Aliases:      []string{"rep"},
		Description:  "Manage saved repeaters",
		CompleteArgs: completeRepeatersArg,
		Children: []CommandSpec{
			{Name: "list", Description: "List saved repeaters"},
			{Name: "add", Description: "Save/login to a repeater", CompleteArgs: completeRepeatersArg},
			{Name: "del", Description: "Remove a saved repeater", CompleteArgs: completeRepeatersArg},
			{Name: "delete", Description: "Remove a saved repeater", CompleteArgs: completeRepeatersArg},
			{Name: "remove", Description: "Remove a saved repeater", CompleteArgs: completeRepeatersArg},
			{Name: "status", Description: "Query repeater status", CompleteArgs: completeRepeatersArg},
			{Name: "neighbours", Description: "List repeater neighbours", CompleteArgs: completeRepeatersArg},
			{Name: "neighbors", Description: "List repeater neighbours", CompleteArgs: completeRepeatersArg},
			{Name: "exec", Description: "Run repeater command", CompleteArgs: completeRepeatersArg},
		},
		Run: cmdRepeater,
	},
	{
		Name:         "use",
		Description:  "Set the default device profile",
		CompleteArgs: completeDeviceProfilesArg,
		Run: func(_ context.Context, e *env) error {
			return cmdUse(e)
		},
	},
	{
		Name:        "device",
		Aliases:     []string{"d", "ls", "list", "devices"},
		Description: "Manage saved device profiles",
		Children: []CommandSpec{
			{Name: "list", Description: "List saved profiles", Flags: []FlagSpec{{Name: "--wide", Description: "Wide output"}}},
			{Name: "show", Description: "Show a profile", CompleteArgs: completeDeviceProfilesArg},
			{Name: "remove", Description: "Remove a profile", CompleteArgs: completeDeviceProfilesArg},
		},
		Run: cmdDevice,
	},
	{
		Name:        "session",
		Description: "Manage device sessions (live radio connections)",
		Children: []CommandSpec{
			{Name: "list", Description: "Show device session states"},
			{Name: "start", Description: "Connect a device session", CompleteArgs: completeDeviceProfilesArg},
			{Name: "stop", Description: "Disconnect a device session", CompleteArgs: completeDeviceProfilesArg},
			{Name: "restart", Description: "Reconnect a device session", CompleteArgs: completeDeviceProfilesArg},
		},
		Run: cmdSession,
	},
	{
		Name:        "state",
		Description: "Inspect per-device local state",
		Children: []CommandSpec{
			{Name: "list", Description: "List per-device local-state databases"},
			{Name: "show", Description: "Show one device's local state", CompleteArgs: completeDeviceProfilesArg},
			{Name: "purge", Description: "Delete one device's local state", CompleteArgs: completeDeviceProfilesArg},
			{Name: "prune", Description: "Delete state older than a duration", Flags: []FlagSpec{{Name: "--older-than", Description: "Age threshold", TakesValue: true}}},
		},
		Run: func(_ context.Context, e *env) error {
			return cmdState(e)
		},
	},
	{
		Name:        "config",
		Aliases:     []string{"conf"},
		Description: "Show configuration",
		Children: []CommandSpec{
			{Name: "path", Description: "Print config file path"},
			{Name: "show", Description: "Print current configuration"},
		},
		Run: func(_ context.Context, e *env) error {
			return cmdConfig(e)
		},
	},
	{
		Name:        "connect",
		Aliases:     []string{"add"},
		Description: "Discover or connect to a radio",
		Flags: []FlagSpec{
			{Name: "--usb", Description: "Scan USB serial only"},
			{Name: "--ble", Description: "Scan BLE only"},
			{Name: "--as", Description: "Profile name", TakesValue: true},
			{Name: "--no-save", Description: "Connect without saving"},
		},
		Run: cmdConnect,
	},
	{
		Name:        "raw",
		Description: "Send raw bytes to the radio",
		Run:         cmdRaw,
	},
	{
		Name:        "version",
		Description: "Show version information",
		Run: func(_ context.Context, e *env) error {
			return cmdVersion(e)
		},
	},
	{
		Name:        "help",
		Aliases:     []string{"h", "?"},
		Description: "Show command help",
	},
}

func completeHelpItems(args []string, endsWithSpace bool) []CompletionItem {
	if completingArgIndex(args, endsWithSpace) != 0 {
		return nil
	}
	prefix := completionPrefix(args, endsWithSpace)
	var items []CompletionItem
	for _, spec := range commandRegistry {
		if spec.Name == "help" {
			continue
		}
		if prefix != "" && !stringsHasPrefixFold(spec.Name, prefix) {
			continue
		}
		items = append(items, CompletionItem{Value: spec.Name, Description: spec.Description})
	}
	return items
}

func findCommandSpec(specs []CommandSpec, name string) (CommandSpec, bool) {
	for _, spec := range specs {
		if spec.Name == name {
			return spec, true
		}
		for _, alias := range spec.Aliases {
			if alias == name {
				return spec, true
			}
		}
	}
	return CommandSpec{}, false
}
