package whatsapp

import (
	"strconv"
	"strings"
)

// Command is one parsed bot command.
type Command struct {
	Name string
	// Args are the whitespace-separated words after the command.
	Args []string
	// Rest is the raw text after the command word, newlines preserved, for
	// commands that take names or one entry per line.
	Rest string
}

// noArgCommands are only recognized when the message is exactly the command
// word, so conversation such as "next week?" or "stop it lol" never acts.
var noArgCommands = map[string]bool{
	"help": true, "status": true, "players": true, "matches": true,
	"members": true, "start": true, "stop": true, "pause": true,
	"resume": true, "next": true, "cancel": true,
}

// argCommands take arguments; malformed arguments get a usage reply.
var argCommands = map[string]bool{
	"add": true, "remove": true, "checkin": true, "roster": true,
	"grade": true, "mode": true, "suggest": true, "assign": true,
	"setup": true,
}

// ParseCommand recognizes a bot command in a chat message. ok is false for
// ordinary conversation, which the bot ignores silently.
func ParseCommand(text string) (cmd Command, ok bool) {
	text = strings.TrimSpace(text)
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return Command{}, false
	}
	name := strings.ToLower(fields[0])
	args := fields[1:]
	rest := strings.TrimSpace(text[len(fields[0]):])

	if noArgCommands[name] {
		return Command{Name: name}, len(args) == 0
	}
	if argCommands[name] {
		return Command{Name: name, Args: args, Rest: rest}, true
	}
	switch name {
	case "courts":
		// "courts are full" is conversation, "courts 3" is a command.
		if len(args) != 1 || !isInt(args[0]) {
			return Command{}, false
		}
		return Command{Name: name, Args: args, Rest: rest}, true
	case "done":
		// done <court> <scoreA>-<scoreB>  |  done <court> <scoreA> <scoreB>
		if _, _, _, ok := parseDone(args); !ok {
			return Command{}, false
		}
		return Command{Name: name, Args: args, Rest: rest}, true
	}
	return Command{}, false
}

// parseDone extracts the court number and scores from a "done" command.
func parseDone(args []string) (court int, scoreA, scoreB int32, ok bool) {
	var a, b string
	switch len(args) {
	case 2:
		var found bool
		a, b, found = strings.Cut(args[1], "-")
		if !found {
			return 0, 0, 0, false
		}
	case 3:
		a, b = args[1], args[2]
	default:
		return 0, 0, 0, false
	}
	court, err := strconv.Atoi(args[0])
	if err != nil || court < 1 {
		return 0, 0, 0, false
	}
	sa, errA := strconv.ParseInt(a, 10, 32)
	sb, errB := strconv.ParseInt(b, 10, 32)
	if errA != nil || errB != nil || sa < 0 || sb < 0 {
		return 0, 0, 0, false
	}
	return court, int32(sa), int32(sb), true
}

// splitNames splits a list of player names. Commas or new lines separate
// names that contain spaces ("Mary Jane, Bob"); otherwise every word is a
// name ("Alice Bob Cara").
func splitNames(s string) []string {
	var parts []string
	if strings.ContainsAny(s, ",\n") {
		parts = strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' })
	} else {
		parts = strings.Fields(s)
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.Join(strings.Fields(p), " "); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseAssign splits "1 Alice Bob vs Cara Dan" into a court number and two
// teams of two names.
func parseAssign(rest string) (court int, teamA, teamB [2]string, ok bool) {
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return 0, teamA, teamB, false
	}
	court, err := strconv.Atoi(fields[0])
	if err != nil || court < 1 {
		return 0, teamA, teamB, false
	}
	body := strings.TrimSpace(rest[strings.Index(rest, fields[0])+len(fields[0]):])
	var left, right string
	found := false
	words := strings.Fields(body)
	for i, w := range words {
		if strings.EqualFold(w, "vs") {
			left, right, found = strings.Join(words[:i], " "), strings.Join(words[i+1:], " "), true
			break
		}
	}
	if !found {
		return 0, teamA, teamB, false
	}
	a, b := splitNames(left), splitNames(right)
	if len(a) != 2 || len(b) != 2 {
		return 0, teamA, teamB, false
	}
	return court, [2]string{a[0], a[1]}, [2]string{b[0], b[1]}, true
}

// memberLine is one "Name [M/F] [A-E]" entry from `add` or `roster`.
type memberLine struct {
	Name   string
	Gender string // "M", "F" or ""
	Grade  string // "A".."E" or ""
}

// parseMemberLine reads trailing gender and grade tokens (in either order)
// off a name: "Mary Jane F B" → Mary Jane, F, B.
func parseMemberLine(s string) memberLine {
	words := strings.Fields(s)
	var ml memberLine
	for len(words) > 1 {
		last := strings.ToUpper(words[len(words)-1])
		switch {
		case ml.Gender == "" && (last == "M" || last == "F"):
			ml.Gender = last
		case ml.Grade == "" && len(last) == 1 && last >= "A" && last <= "E":
			ml.Grade = last
		default:
			ml.Name = strings.Join(words, " ")
			return ml
		}
		words = words[:len(words)-1]
	}
	ml.Name = strings.Join(words, " ")
	return ml
}

func isInt(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}
