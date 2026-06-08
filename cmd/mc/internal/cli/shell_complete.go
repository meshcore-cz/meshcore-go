package cli

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"

	meshcore "github.com/meshcore-cz/meshcore-go"
	localbackend "github.com/meshcore-cz/meshcore-go/backend"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/config"
	"github.com/reeflective/readline"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type CompletionItem struct {
	Value       string
	Description string
}

type shellCompletionCache struct {
	mu sync.Mutex

	contacts  []CompletionItem
	channels  []CompletionItem
	devices   []CompletionItem
	repeaters []CompletionItem
}

func (c *shellCompletionCache) contactItems(ctx context.Context, session *shellSession) []CompletionItem {
	c.mu.Lock()
	if c.contacts != nil {
		items := c.contacts
		c.mu.Unlock()
		return items
	}
	c.mu.Unlock()

	client := localbackend.NewClient(session.Socket)
	contacts, err := client.ContactsWithOptions(ctx, true, false, false, false)
	if err != nil {
		return nil
	}

	items := make([]CompletionItem, 0, len(contacts))
	for _, ct := range contacts {
		items = append(items, CompletionItem{
			Value:       quoteCompletion(ct.Name),
			Description: humanContactType(ct.Type),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Value < items[j].Value
	})

	c.mu.Lock()
	c.contacts = items
	c.mu.Unlock()
	return items
}

func (c *shellCompletionCache) channelItems(ctx context.Context, session *shellSession) []CompletionItem {
	c.mu.Lock()
	if c.channels != nil {
		items := c.channels
		c.mu.Unlock()
		return items
	}
	c.mu.Unlock()

	client := localbackend.NewClient(session.Socket)
	channels, err := client.ChannelsWithOptions(ctx, false)
	if err != nil {
		return nil
	}

	items := make([]CompletionItem, 0, len(channels))
	for _, ch := range channels {
		value := ch.Name
		if value == "" {
			value = strconv.Itoa(int(ch.Index))
		}
		items = append(items, CompletionItem{
			Value:       quoteCompletion(value),
			Description: "channel",
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Value < items[j].Value
	})

	c.mu.Lock()
	c.channels = items
	c.mu.Unlock()
	return items
}

func (c *shellCompletionCache) deviceItems() []CompletionItem {
	c.mu.Lock()
	if c.devices != nil {
		items := c.devices
		c.mu.Unlock()
		return items
	}
	c.mu.Unlock()

	cfg, err := config.Load()
	if err != nil {
		return nil
	}

	items := make([]CompletionItem, 0, len(cfg.Devices))
	for name := range cfg.Devices {
		desc := "device profile"
		if cfg.Current == name {
			desc = "current profile"
		}
		items = append(items, CompletionItem{Value: name, Description: desc})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Value < items[j].Value
	})

	c.mu.Lock()
	c.devices = items
	c.mu.Unlock()
	return items
}

func (c *shellCompletionCache) repeaterItems() []CompletionItem {
	c.mu.Lock()
	if c.repeaters != nil {
		items := c.repeaters
		c.mu.Unlock()
		return items
	}
	c.mu.Unlock()

	cfg, err := config.Load()
	if err != nil {
		return nil
	}

	items := make([]CompletionItem, 0, len(cfg.Repeaters))
	for key, rep := range cfg.Repeaters {
		name := rep.Name
		if name == "" {
			name = key
		}
		items = append(items, CompletionItem{Value: quoteCompletion(name), Description: "repeater profile"})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Value < items[j].Value
	})

	c.mu.Lock()
	c.repeaters = items
	c.mu.Unlock()
	return items
}

func completeContactsArg(ctx context.Context, session *shellSession, args []string, endsWithSpace bool) []CompletionItem {
	if completingArgIndex(args, endsWithSpace) != 0 {
		return nil
	}
	cache := shellCacheFromSession(session)
	return filterCompletionItems(cache.contactItems(ctx, session), completionPrefix(args, endsWithSpace))
}

func completeChannelsArg(ctx context.Context, session *shellSession, args []string, endsWithSpace bool) []CompletionItem {
	if completingArgIndex(args, endsWithSpace) != 0 {
		return nil
	}
	cache := shellCacheFromSession(session)
	return filterCompletionItems(cache.channelItems(ctx, session), completionPrefix(args, endsWithSpace))
}

func completeDeviceProfilesArg(_ context.Context, session *shellSession, args []string, endsWithSpace bool) []CompletionItem {
	if completingArgIndex(args, endsWithSpace) != 0 {
		return nil
	}
	cache := shellCacheFromSession(session)
	return filterCompletionItems(cache.deviceItems(), completionPrefix(args, endsWithSpace))
}

func completeRepeatersArg(_ context.Context, session *shellSession, args []string, endsWithSpace bool) []CompletionItem {
	if completingArgIndex(args, endsWithSpace) != 0 {
		return nil
	}
	cache := shellCacheFromSession(session)
	return filterCompletionItems(cache.repeaterItems(), completionPrefix(args, endsWithSpace))
}

func shellCacheFromSession(session *shellSession) *shellCompletionCache {
	if session.completion == nil {
		session.completion = &shellCompletionCache{}
	}
	return session.completion
}

func makeShellCompleter(session *shellSession) func([]rune, int) readline.Completions {
	return func(line []rune, cursor int) readline.Completions {
		if cursor < 0 {
			cursor = len(line)
		}
		if cursor > len(line) {
			cursor = len(line)
		}

		input := string(line[:cursor])
		words := completionWords(input)
		endsWithSpace := strings.HasSuffix(input, " ") || strings.HasSuffix(input, "\t")

		root := NewRoot(&App{})
		if len(words) == 0 || (len(words) == 1 && !endsWithSpace) {
			return completeCobraCommands(root.Commands())
		}

		cmd, ok := findCobraCommand(root, words[0])
		if !ok {
			return readline.Completions{}
		}

		return completeForCobraCommand(context.Background(), session, cmd, words[1:], input, endsWithSpace)
	}
}

func completeForCobraCommand(
	ctx context.Context,
	session *shellSession,
	cmd *cobra.Command,
	rest []string,
	input string,
	endsWithSpace bool,
) readline.Completions {
	if len(rest) > 0 {
		last := rest[len(rest)-1]
		if strings.HasPrefix(last, "-") {
			return completeCobraFlags(cmd, last)
		}
	}

	children := visibleCommands(cmd)
	if len(children) > 0 {
		if len(rest) == 0 || (len(rest) == 1 && !endsWithSpace) {
			return completeCobraCommands(children)
		}
		child, ok := findCobraCommand(cmd, rest[0])
		if !ok {
			return readline.Completions{}
		}
		return completeForCobraCommand(ctx, session, child, rest[1:], input, endsWithSpace)
	}

	if items := completeDomainArgs(ctx, session, cmd, rest, endsWithSpace); items != nil {
		return completionItems(items)
	}

	if cmd.ValidArgsFunction != nil {
		prefix := completionPrefix(rest, endsWithSpace)
		values, _ := cmd.ValidArgsFunction(cmd, rest, prefix)
		return completionItems(cobraCompletionItems(values, prefix))
	}

	if cmd.Name() == "help" {
		return completeCobraCommands(cmd.Root().Commands())
	}

	if len(rest) == 0 || endsWithSpace {
		return completeCobraFlags(cmd, "--")
	}

	return readline.Completions{}
}

func completeDomainArgs(ctx context.Context, session *shellSession, cmd *cobra.Command, rest []string, endsWithSpace bool) []CompletionItem {
	switch cmd.CommandPath() {
	case "mc contact show", "mc send", "mc trace":
		return completeContactsArg(ctx, session, rest, endsWithSpace)
	case "mc channel show", "mc channel send", "mc channel remove":
		return completeChannelsArg(ctx, session, rest, endsWithSpace)
	case "mc use", "mc device show", "mc device remove", "mc session start", "mc session stop", "mc session restart", "mc state show", "mc state purge":
		return completeDeviceProfilesArg(ctx, session, rest, endsWithSpace)
	case "mc repeater add", "mc repeater del", "mc repeater status", "mc repeater neighbours", "mc repeater exec":
		return completeRepeatersArg(ctx, session, rest, endsWithSpace)
	default:
		return nil
	}
}

func findCobraCommand(parent *cobra.Command, name string) (*cobra.Command, bool) {
	for _, cmd := range parent.Commands() {
		if cmd.Hidden {
			continue
		}
		if cmd.Name() == name {
			return cmd, true
		}
		for _, alias := range cmd.Aliases {
			if alias == name {
				return cmd, true
			}
		}
	}
	return nil, false
}

func visibleCommands(cmd *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	for _, child := range cmd.Commands() {
		if !child.Hidden {
			out = append(out, child)
		}
	}
	return out
}

func completeCobraCommands(commands []*cobra.Command) readline.Completions {
	values := make([]string, 0, len(commands)*4)
	for _, cmd := range commands {
		if cmd.Hidden {
			continue
		}
		values = append(values, cmd.Name(), cmd.Short)
		for _, alias := range cmd.Aliases {
			values = append(values, alias, "alias for "+cmd.Name())
		}
	}
	return readline.CompleteValuesDescribed(values...).DisplayList().JustifyDescriptions()
}

func completeCobraFlags(cmd *cobra.Command, prefix string) readline.Completions {
	values := []string{}
	add := func(flag *pflag.Flag) {
		name := "--" + flag.Name
		if prefix != "" && prefix != "--" && !strings.HasPrefix(name, prefix) {
			return
		}
		values = append(values, name, flag.Usage)
		if flag.Shorthand != "" {
			short := "-" + flag.Shorthand
			if prefix == "" || prefix == "--" || strings.HasPrefix(short, prefix) {
				values = append(values, short, flag.Usage)
			}
		}
	}
	cmd.InheritedFlags().VisitAll(add)
	cmd.Flags().VisitAll(add)
	if len(values) == 0 {
		return readline.Completions{}
	}
	return readline.CompleteValuesDescribed(values...).DisplayList().JustifyDescriptions()
}

func cobraCompletionItems(values []string, prefix string) []CompletionItem {
	items := make([]CompletionItem, 0, len(values))
	for _, value := range values {
		item, desc, _ := strings.Cut(value, "\t")
		if prefix != "" && !stringsHasPrefixFold(strings.Trim(item, `"'`), prefix) {
			continue
		}
		items = append(items, CompletionItem{Value: item, Description: desc})
	}
	return items
}

func completionItems(items []CompletionItem) readline.Completions {
	if len(items) == 0 {
		return readline.Completions{}
	}
	values := make([]string, 0, len(items)*2)
	for _, item := range items {
		desc := item.Description
		if desc == "" {
			desc = item.Value
		}
		values = append(values, item.Value, desc)
	}
	return readline.CompleteValuesDescribed(values...).DisplayList().JustifyDescriptions()
}

func completionWords(input string) []string {
	var words []string
	var current strings.Builder
	var quote rune
	escaped := false

	flush := func() {
		if current.Len() == 0 {
			return
		}
		words = append(words, current.String())
		current.Reset()
	}

	for _, r := range input {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case r == ' ' || r == '\t':
			flush()
		default:
			current.WriteRune(r)
		}
	}

	flush()
	return words
}

func completingArgIndex(args []string, endsWithSpace bool) int {
	if len(args) == 0 {
		return 0
	}
	if endsWithSpace {
		return len(args)
	}
	return len(args) - 1
}

func completionPrefix(args []string, endsWithSpace bool) string {
	if len(args) == 0 {
		return ""
	}
	if endsWithSpace {
		return ""
	}
	return args[len(args)-1]
}

func filterCompletionItems(items []CompletionItem, prefix string) []CompletionItem {
	if prefix == "" {
		return items
	}
	out := make([]CompletionItem, 0, len(items))
	for _, item := range items {
		value := strings.Trim(item.Value, `"'`)
		if stringsHasPrefixFold(value, prefix) {
			out = append(out, item)
		}
	}
	return out
}

func quoteCompletion(value string) string {
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, " \t\"'\\") {
		return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	}
	return value
}

func humanContactType(t meshcore.ContactType) string {
	switch t {
	case meshcore.ContactChat:
		return "companion"
	case meshcore.ContactRepeater:
		return "repeater"
	case meshcore.ContactRoom:
		return "room"
	case meshcore.ContactSensor:
		return "sensor"
	default:
		return "contact"
	}
}

func stringsHasPrefixFold(s, prefix string) bool {
	return len(prefix) <= len(s) && strings.EqualFold(s[:len(prefix)], prefix)
}
