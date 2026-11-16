package whatsapp

import "testing"

func TestParseCommand(t *testing.T) {
	cases := []struct {
		text  string
		ok    bool
		name  string
		nArgs int
	}{
		{"next", true, "next", 0},
		{"  NEXT  ", true, "next", 0},
		{"status", true, "status", 0},
		// Conversation that merely starts with a command word is ignored.
		{"next week?", false, "", 0},
		{"stop it lol", false, "", 0},
		{"start at 7pm", false, "", 0},
		{"see you tonight", false, "", 0},
		{"", false, "", 0},
		{"add Mary Jane F", true, "add", 3},
		{"add", true, "add", 0},
		{"remove Bob", true, "remove", 1},
		{"courts 3", true, "courts", 1},
		{"courts are full", false, "", 0},
		{"done 2 21-15", true, "done", 2},
		{"done 2 21 15", true, "done", 3},
		{"done for today", false, "", 0},
		{"done 2", false, "", 0},
		{"done 0 21-15", false, "", 0},
		{"done 2 -1-15", false, "", 0},
	}
	for _, tc := range cases {
		cmd, ok := ParseCommand(tc.text)
		if ok != tc.ok {
			t.Errorf("ParseCommand(%q) ok = %v, want %v", tc.text, ok, tc.ok)
			continue
		}
		if ok && (cmd.Name != tc.name || len(cmd.Args) != tc.nArgs) {
			t.Errorf("ParseCommand(%q) = %+v, want name %q with %d args", tc.text, cmd, tc.name, tc.nArgs)
		}
	}
}

func TestParseDone(t *testing.T) {
	court, a, b, ok := parseDone([]string{"3", "21-19"})
	if !ok || court != 3 || a != 21 || b != 19 {
		t.Fatalf("got court=%d a=%d b=%d ok=%v", court, a, b, ok)
	}
	court, a, b, ok = parseDone([]string{"1", "15", "21"})
	if !ok || court != 1 || a != 15 || b != 21 {
		t.Fatalf("got court=%d a=%d b=%d ok=%v", court, a, b, ok)
	}
}
