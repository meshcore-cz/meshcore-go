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
}

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

var shellCommands = []CommandSpec{
	{
		Name:        "status",
		Aliases:     []string{"s"},
		Description: "Show device status",
		Flags: []FlagSpec{
			{Name: "--live", Description: "Refresh radio stats before showing status"},
		},
	},
	{
		Name:        "stats",
		Description: "Show local radio statistics",
	},
	{
		Name:        "doctor",
		Description: "Run connection diagnostics",
	},
	{
		Name:        "contacts",
		Aliases:     []string{"c"},
		Description: "List contacts",
		Flags: []FlagSpec{
			{Name: "--wide", Description: "Show paths and coordinates"},
			{Name: "--refresh", Description: "Refresh contact replica"},
			{Name: "--wait", Description: "Wait for refresh to finish"},
			{Name: "--full", Description: "Full contact sync"},
			{Name: "--cached", Description: "Read local replica only"},
			{Name: "--type", Description: "Filter by contact type", TakesValue: true},
			{Name: "--route", Description: "Filter by route kind", TakesValue: true},
			{Name: "--within", Description: "Filter by distance", TakesValue: true},
			{Name: "--sort", Description: "Sort field", TakesValue: true},
		},
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
	},
	{
		Name:        "inbox",
		Description: "Drain buffered incoming messages",
	},
	{
		Name:         "send",
		Description:  "Send a direct message",
		CompleteArgs: completeContactsArg,
		Flags: []FlagSpec{
			{Name: "--wait", Description: "Wait for acknowledgement"},
		},
	},
	{
		Name:         "trace",
		Description:  "Trace route to a contact or path",
		CompleteArgs: completeContactsArg,
	},
	{
		Name:        "channel",
		Description: "List or use channels",
		Children: []CommandSpec{
			{
				Name:        "list",
				Description: "List channels",
				Flags: []FlagSpec{
					{Name: "--refresh", Description: "Refresh channel replica"},
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
		},
	},
	{
		Name:        "advert",
		Description: "Broadcast this device's advert",
		Flags: []FlagSpec{
			{Name: "--flood", Description: "Flood advert mesh-wide"},
		},
	},
	{
		Name:        "discover",
		Description: "Scan for nearby nodes",
	},
	{
		Name:        "watch",
		Description: "Stream incoming messages and events",
		Flags: []FlagSpec{
			{Name: "--raw", Description: "Stream raw packets as JSON"},
		},
	},
	{
		Name:        "backend",
		Aliases:     []string{"b"},
		Description: "Manage the local backend",
		Children: []CommandSpec{
			{Name: "status", Description: "Show backend diagnostics", Flags: []FlagSpec{{Name: "--verbose", Description: "Verbose backend status"}}},
			{Name: "start", Description: "Start backend"},
			{Name: "stop", Description: "Stop backend"},
			{Name: "restart", Description: "Restart backend", Flags: []FlagSpec{{Name: "--reset", Description: "Reset replica state"}}},
			{Name: "reset", Description: "Stop backend and delete replica state"},
			{Name: "serve", Description: "Run backend in foreground"},
			{Name: "log", Description: "Show backend logs", Flags: []FlagSpec{
				{Name: "--follow", Description: "Follow log output"},
				{Name: "-f", Description: "Follow log output"},
				{Name: "--lines", Description: "Number of lines", TakesValue: true},
				{Name: "-n", Description: "Number of lines", TakesValue: true},
			}},
		},
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
	},
	{
		Name:         "use",
		Description:  "Set the default device profile",
		CompleteArgs: completeDeviceProfilesArg,
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
	},
	{
		Name:        "config",
		Aliases:     []string{"conf"},
		Description: "Show configuration",
		Children: []CommandSpec{
			{Name: "path", Description: "Print config file path"},
			{Name: "show", Description: "Print current configuration"},
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
	},
	{
		Name:        "raw",
		Description: "Send raw bytes to the radio",
	},
	{
		Name:        "version",
		Description: "Show version information",
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
	for _, spec := range shellCommands {
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
	canonical := resolveAlias(name)
	for _, spec := range specs {
		if spec.Name == name || spec.Name == canonical {
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
