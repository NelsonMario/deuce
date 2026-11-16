package whatsapp

import (
	"strconv"
	"strings"
)

// Command is one parsed group command.
type Command struct {
	Name string
	Args []string
}

// noArgCommands are only recognized when the message is exactly the command
// word. The bot shares the group with normal conversation, so an admin
// writing "next week?" or "stop it lol" must not generate a match or end
// the session.
var noArgCommands = map[string]bool{
	"help": true, "status": true, "players": true, "matches": true,
	"start": true, "stop": true, "pause": true, "resume": true,
	"next": true, "cancel": true,
}

// ParseCommand recognizes a group command in a chat message. ok is false for
// ordinary conversation, which the bot ignores silently.
func ParseCommand(text string) (cmd Command, ok bool) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return Command{}, false
	}
	name := strings.ToLower(fields[0])
	args := fields[1:]

	if noArgCommands[name] {
		return Command{Name: name}, len(args) == 0
	}
	switch name {
	case "add", "remove":
		// The name is free text; usage is reported for a bare "add".
		return Command{Name: name, Args: args}, true
	case "courts":
		// "courts are full" is conversation, "courts 3" is a command.
		if len(args) != 1 || !isInt(args[0]) {
			return Command{}, false
		}
		return Command{Name: name, Args: args}, true
	case "done":
		// done <court> <scoreA>-<scoreB>  |  done <court> <scoreA> <scoreB>
		if _, _, _, ok := parseDone(args); !ok {
			return Command{}, false
		}
		return Command{Name: name, Args: args}, true
	}
	return Command{}, false
}

// parseDone extracts the court reference and scores from the arguments of a
// "done" command. The court is a number ("2" for "Court 2").
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

func isInt(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}
