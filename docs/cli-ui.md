Here is a reusable Markdown guide you can keep in `docs/cli-ui.md` or include in your agent instructions.

# CLI UI Design Guidelines

The CLI should feel like a modern Unix tool: compact, predictable, readable in a normal terminal, and useful both for humans and scripts.

The goal is not to build a full-screen TUI. Commands should print structured text that is easy to scan, copy, pipe, and debug.

## General principles

### Prefer information hierarchy over decoration

Use whitespace, alignment, and concise wording before adding colors or symbols.

Good:

```text
Device:        EFF01EF2
Firmware:      MeshCore (Heltec V3) v1.16.0
Transport:     BLE

Radio:
  Modem:       869.432 MHz · BW 62.5 kHz · SF7 · CR 4/5 · TX 22 dBm
  Signal:      -51 dBm RSSI · +12.0 dB SNR · -108 dBm noise
  Battery:     3.90 V
```

Avoid noisy layouts:

```text
=== DEVICE INFORMATION ===
✅ Device Name: EFF01EF2
📡 Firmware Version: MeshCore (Heltec V3) v1.16.0
🔵 Bluetooth Device Connected!
```

Use icons only when they add real meaning. Most command output does not need them.

### Keep the default view practical

The default output should answer the most common operational questions.

Put advanced diagnostics behind:

```sh
--wide
--verbose
--debug
--json
```

Do not make every user read low-level details on every invocation.

### Keep machine-readable output separate

Human output may use aligned columns, relative times, colors, arrows, and compact formatting.

JSON output should remain stable, complete, and unstyled:

```sh
mc status --json
mc contacts --json
mc trace 2525 --json
```

Never parse human-rendered strings back into structured data. Build both human output and JSON from the same typed internal model.

---

## Output structure

Use a small number of repeatable layout patterns.

## Pattern 1: aligned key-value summary

Use for commands such as:

```sh
mc status
mc contact show <name>
mc repeater status <name>
```

Example:

```text
Device:        EFF01EF2
Firmware:      MeshCore (Heltec V3) v1.16.0
Protocol:      companion-v3
Transport:     ble://90d56c84-42ef-36f3-89ae-9e8f42231b00
Public key:    eff01ef21805fb309c2dba6073ac3954511a2b08be41c9bc787cc12a41879aa7

Radio:
  Modem:       869.432 MHz · BW 62.5 kHz · SF7 · CR 4/5 · TX 22 dBm
  Signal:      -51 dBm RSSI · +12.0 dB SNR · -108 dBm noise
  Battery:     3.90 V
  Uptime:      10h 57m
  Packets:     12,846 rx · 1,284 tx · 7 errors
  Airtime:     2h 14m rx · 8m 37s tx
  Queue:       0 pending

Backend:
  State:       ready (pid 3123)
  Replica:     fresh
  Contacts:    449, updated 11m ago
  Channels:    2, updated 11m ago
```

Guidelines:

* align labels;
* group related values under short section headers;
* separate sections with blank lines;
* avoid deeply nested structures;
* use concise nouns such as `State`, `Signal`, `Queue`, and `Packets`.

## Pattern 2: table with footer

Use for list commands such as:

```sh
mc contacts
mc device list
mc repeater list
```

Example:

```text
NAME                       TYPE       SEEN       ADV   ROUTE     DIST      KEY
011111000101               companion  18d ago    -     flood     -         3b46ce7cb2f3
3gy.pt repeater            repeater   42m ago    2B    direct    8.4 km    2690e0d0b5e9
AKAT-Tester1 🗼             repeater   8m ago     2B    3 hops    62 km     57dbabc15c30
Alešova                    repeater   6m ago     1B    1 hop     4.1 km    a90d1408ff61

449 contacts · synced 11m ago
```

Guidelines:

* use uppercase column headers;
* keep column names short;
* sort predictably by default;
* put metadata in a footer, not above the table;
* use `-` for unavailable values;
* keep the footer visually quieter than the data rows;
* add `--wide` for full keys, paths, and coordinates.

## Pattern 3: summary plus table

Use for commands such as:

```sh
mc trace <target>
mc discover
```

Example:

```text
Trace:         [2525] mc.kololec.cz
Request:       explicit path · 2525
Prefix:        2B
Round trip:    687ms

LEG  FROM                    TO                      SNR
───  ──────────────────────  ──────────────────────  ─────────
1    [eff0] EFF01EF2       → [2525] mc.kololec.cz  +12.5 dB
2    [2525] mc.kololec.cz  → [eff0] EFF01EF2       +11.5 dB

2 legs · weakest +11.5 dB
```

Guidelines:

* put the most important context above the table;
* keep the table focused on repeated values;
* put aggregate information in a compact footer;
* prefer human terminology such as `LEG` when `HOP` would be misleading.

---

## Color usage

Use color like Git: sparingly and semantically.

The output should still be fully understandable without color.

### Good uses of color

Use green for explicit healthy state words:

```text
ready
fresh
connected
```

Use yellow for warnings or degraded states:

```text
stale
syncing
-2.0 dB  ← weak
Warning:
```

Use red only for failures:

```text
offline
error
timeout after 5.0s
```

Use dim styling for secondary information:

```text
[2525]
→
table separator
footer metadata
public-key prefixes
-
```

### Avoid excessive color

Do not color:

* every row;
* every node type;
* all positive signal values;
* whole sections;
* full error messages when highlighting one word is enough.

Good:

```text
Backend:       ready
```

Only `ready` is green.

Avoid:

```text
Backend:       ready
```

with the entire line in bright green.

### Disable color correctly

Never print ANSI escape sequences when:

* stdout is redirected;
* output is piped;
* `NO_COLOR` is set;
* JSON output is requested.

Examples:

```sh
mc contacts > contacts.txt
mc contacts | grep repeater
NO_COLOR=1 mc trace 2525
mc status --json
```

These must produce plain output.

---

## Naming and wording

Prefer short, precise labels.

Good:

```text
Radio:
  Modem:
  Signal:
  Battery:
  Uptime:
  Packets:
  Airtime:
  Queue:
```

Avoid vague or verbose labels:

```text
Radio configuration settings:
Current battery voltage:
Total number of packets received and transmitted:
```

Use consistent terminology across commands.

For example:

```text
ADV       advert path-hash width
ROUTE     flood, direct, or hop count
DIST      geographical straight-line distance
KEY       shortened public-key prefix
LEG       one measured directional trace link
```

Do not use different words for the same concept in different commands.

---

## Identifiers before names

When a CLI displays network nodes, prefer stable identifiers first and human names second.

Good:

```text
[2525] mc.kololec.cz
[a90d] Alešova
[3f18]
```

Avoid:

```text
mc.kololec.cz [2525]
Alešova [a90d]
```

Why:

* the identifier is the protocol-level value;
* the human name may be missing;
* the human name may be ambiguous;
* hash-first output aligns better;
* unresolved nodes still look consistent.

For unresolved nodes:

```text
[3f18]
```

For ambiguous matches, show only the identifier in the table and print a warning below:

```text
Warning: prefix [25] matches multiple contacts:
  mc.kololec.cz
  mc.koren.cz
```

---

## Relative time

Use relative time for list views:

```text
now
42m ago
8h ago
18d ago
never
```

Use both relative and absolute timestamps in detail views:

```text
Last seen:     8m ago  (2026-06-07 14:32:08)
```

Avoid overly precise values inside compact tables unless the precision is useful.

Good:

```text
42m ago
```

Avoid:

```text
42m 18s ago
```

---

## Numbers and units

Keep units compact and consistent.

### Distance

```text
740 m
4.1 km
62 km
```

Rules:

* below `1 km`: metres;
* below `10 km`: one decimal place;
* above `10 km`: rounded kilometres.

### Signal

```text
+12.5 dB
-2.0 dB
-108 dBm
```

Rules:

* always include sign for SNR;
* use one decimal place for SNR;
* use integer values for RSSI and noise floor unless additional precision is useful.

### Duration

```text
687ms
1.42s
10h 57m
```

Use compact durations. Avoid unnecessary spaces inside machine-like timing values:

```text
687ms
```

not:

```text
687 ms
```

### Counts

Use separators for large values:

```text
12,846
```

not:

```text
12846
```

---

## Wide mode

Default output should fit comfortably in a normal terminal.

Use:

```sh
mc contacts --wide
```

for diagnostic data:

```text
NAME              TYPE       SEEN     ADV   ROUTE    PATH                 DIST    LOCATION               KEY
Alešova           repeater   6m ago   1B    1 hop    a9                   4.1 km  50.479238, 13.990112   a90d1408ff61103e...
AKAT-Tester1 🗼    repeater   8m ago   2B    3 hops   a90d → 57db → 3f18   62 km   49.195100, 16.606800   57dbabc15c30eb8d...
```

Wide mode is appropriate for:

* complete public keys;
* raw paths;
* exact coordinates;
* timestamps;
* implementation details.

Keep the default table focused.

---

## Exceptional states

Normal successful output should stay quiet.

Show warnings only when the user needs to notice something.

Example:

```text
Warning: showing cached contacts; backend is offline.
```

Then render the normal table:

```text
NAME                       TYPE       SEEN       ADV   ROUTE     DIST      KEY
...
```

Footer:

```text
449 contacts · cached · last synced 10h 56m ago
```

For an active refresh:

```text
449 contacts · syncing 183/449 · last synced 11m ago
```

For missing data, prefer:

```text
-
```

rather than:

```text
unknown
N/A
not available
none
```

Use an explicit word only when it carries useful meaning:

```text
flood
direct
never
offline
```

---

## Unicode and alignment

Terminal output may contain:

```text
Alešova
AKAT-Tester1 🗼
AKAT-Room 📬
```

Do not calculate visible widths with:

```go
len(value)
```

because UTF-8 byte length is not terminal display width.

Use a terminal-width-aware helper such as:

```go
runewidth.StringWidth(value)
```

from:

```text
github.com/mattn/go-runewidth
```

Calculate widths from unstyled strings.

Apply ANSI styling only after width calculation, because escape sequences must not affect alignment.

---

## Rendering architecture

Keep data retrieval separate from presentation.

Bad:

```go
fmt.Printf(
    "%-20s %-10s %s\n",
    contact.Name,
    contact.Type,
    colorize(routeLabel(contact)),
)
```

scattered throughout command handlers.

Better:

```go
type ContactRow struct {
    Name      string
    Type      string
    Seen      string
    Advert    string
    Route     string
    Distance string
    Key       string
}
```

Then render through a dedicated UI package:

```go
ui.RenderContacts(data, printer)
```

Recommended split:

```text
cmd/mc/internal/cli/
  contacts.go       backend calls, SDK adaptation, flags, JSON

cmd/mc/internal/ui/
  contacts.go       table rendering
  trace.go          trace rendering
  status.go         status rendering
  theme.go          ANSI styling
  displaywidth.go   Unicode-safe padding
```

The UI package should receive prepared view models. It should not make backend calls or depend on protocol internals.

---

## Structured models, not rendered-string parsing

Do not store structured data as a formatted string.

Bad:

```go
label := "mc.kololec.cz [2525]"
```

and later:

```go
name, hash := parseLabel(label)
```

Better:

```go
type TraceNode struct {
    Hash      string
    Name      string
    Ambiguous bool
    Names     []string
}
```

Render when needed:

```go
func (n TraceNode) Label() string {
    if n.Name == "" {
        return "[" + n.Hash + "]"
    }

    return "[" + n.Hash + "] " + n.Name
}
```

Generate JSON directly from the struct.

Human output and JSON output should never depend on parsing each other.

---

## Footer design

Use compact footers to summarize result sets.

Good:

```text
449 contacts · synced 11m ago
```

```text
6 legs · weakest -2.0 dB
```

```text
12 nodes · 3 repeaters · 2 rooms · completed in 4.2s
```

Footer rules:

* keep it to one line when possible;
* render it dimly;
* place it after the data;
* separate it with one blank line;
* avoid repeating information already visible in the table.

---

## Tables should remain pipe-friendly

Human-readable output should still behave like a normal Unix tool.

Commands should work reasonably with:

```sh
mc contacts | grep repeater
mc contacts | less
mc contacts > contacts.txt
```

Avoid:

* animated spinners in non-interactive mode;
* cursor movement;
* hidden redraws;
* full-screen terminal modes;
* decorative borders around every table cell.

Good:

```text
NAME                       TYPE       SEEN       ADV   ROUTE     DIST      KEY
Alešova                    repeater   6m ago     1B    1 hop     4.1 km    a90d1408ff61
```

Avoid:

```text
┌──────────────────────────┬───────────┬──────────┐
│ NAME                     │ TYPE      │ SEEN     │
├──────────────────────────┼───────────┼──────────┤
│ Alešova                  │ repeater  │ 6m ago   │
└──────────────────────────┴───────────┴──────────┘
```

Borders usually add visual weight without adding information.

---

## Progress output

For foreground operations, show concise progress only when useful.

Example:

```text
Syncing contacts: 183/449
```

For background operations, return quickly:

```text
Contact synchronization started.
```

Then expose progress through status:

```text
Backend:
  Replica:     syncing
  Contacts:    183/449 received
```

Do not print rapidly changing progress lines when stdout is redirected.

---

## Recommended style contract

Across the CLI:

```text
section headers       plain or subtle emphasis
labels                plain
ordinary values       plain
stable identifiers    dim
arrows                 dim
table separators      dim
footers                dim
healthy state words   green
warnings               yellow
weak SNR               yellow
errors                 red
missing values         dim "-"
```

This keeps output modern without making it visually noisy.

---

## Example: complete command family

### Status

```text
Device:        EFF01EF2
Firmware:      MeshCore (Heltec V3) v1.16.0
Protocol:      companion-v3
Transport:     BLE

Radio:
  Modem:       869.432 MHz · BW 62.5 kHz · SF7 · CR 4/5 · TX 22 dBm
  Signal:      -51 dBm RSSI · +12.0 dB SNR · -108 dBm noise
  Battery:     3.90 V
  Uptime:      10h 57m
  Packets:     12,846 rx · 1,284 tx · 7 errors
  Airtime:     2h 14m rx · 8m 37s tx
  Queue:       0 pending

Backend:
  State:       ready (pid 3123)
  Replica:     fresh
  Contacts:    449, updated 11m ago
  Channels:    2, updated 11m ago
```

### Contacts

```text
NAME                       TYPE       SEEN       ADV   ROUTE     DIST      KEY
011111000101               companion  18d ago    -     flood     -         3b46ce7cb2f3
3gy.pt repeater            repeater   42m ago    2B    direct    8.4 km    2690e0d0b5e9
AKAT-Tester1 🗼             repeater   8m ago     2B    3 hops    62 km     57dbabc15c30
Alešova                    repeater   6m ago     1B    1 hop     4.1 km    a90d1408ff61

449 contacts · synced 11m ago
```

### Trace

```text
Trace:         [2525] mc.kololec.cz
Request:       explicit path · 2525
Prefix:        2B
Round trip:    687ms

LEG  FROM                    TO                      SNR
───  ──────────────────────  ──────────────────────  ─────────
1    [eff0] EFF01EF2       → [2525] mc.kololec.cz  +12.5 dB
2    [2525] mc.kololec.cz  → [eff0] EFF01EF2       +11.5 dB

2 legs · weakest +11.5 dB
```

These commands use the same visual language:

* key-value summaries for single objects;
* clean tables for lists;
* dim footers for metadata;
* restrained color for state;
* stable identifiers before optional names;
* JSON for automation.
