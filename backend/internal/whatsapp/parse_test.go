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
		{"in Andi Budi", true, "in", 2},
		{"in", false, "", 0},
		{"out Bob", true, "out", 1},
		{"out", false, "", 0},
		{"courts 3", true, "courts", 1},
		{"courts", true, "courts", 0},
		{"courts are full", false, "", 0},
		{"done 2 21-15", true, "done", 2},
		{"done 2 21 15", true, "done", 3},
		{"done for today", false, "", 0},
		{"done 2", false, "", 0},
		{"done 0 21-15", false, "", 0},
		{"done 2 -1-15", false, "", 0},
		{"auto", true, "auto", 0},
		{"manual", true, "manual", 0},
		{"manual please", false, "", 0},
		{"member", true, "member", 0},
		{"member Budi", true, "member", 1},
		{"grade Budi B", true, "grade", 2},
		{"assign 1 Alice Bob vs Cara Dan", true, "assign", 6},
		{"setup admin Tiger Club", true, "setup", 3},
		// Old names keep working as hidden aliases.
		{"add Mary Jane F", true, "in", 3},
		{"add", false, "", 0},
		{"checkin Alice Bob", true, "in", 2},
		{"remove Bob", true, "out", 1},
		{"players", true, "status", 0},
		{"matches", true, "status", 0},
		{"members", true, "member", 0},
		{"members please", false, "", 0},
		{"roster", true, "member", 0},
		{"pause", true, "manual", 0},
		{"resume", true, "auto", 0},
		{"mode manual", true, "manual", 0},
		{"mode auto", true, "auto", 0},
		{"mode of transport", false, "", 0},
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

func TestParseAssign(t *testing.T) {
	court, a, b, ok := parseAssign("2 Mary Jane, Bob vs Cara Dan")
	if !ok || court != 2 || a != [2]string{"Mary Jane", "Bob"} || b != [2]string{"Cara", "Dan"} {
		t.Fatalf("got %d %v %v %v", court, a, b, ok)
	}
	if _, _, _, ok := parseAssign("1 Alice Bob Cara Dan"); ok {
		t.Fatal("missing vs should fail")
	}
	if _, _, _, ok := parseAssign("x Alice Bob vs Cara Dan"); ok {
		t.Fatal("non-numeric court should fail")
	}
}

func TestParseMemberLine(t *testing.T) {
	cases := map[string]memberLine{
		"Alice F A":     {Name: "Alice", Gender: "F", Grade: "A"},
		"Mary Jane B F": {Name: "Mary Jane", Gender: "F", Grade: "B"},
		"Bob":           {Name: "Bob"},
		"Bob m":         {Name: "Bob", Gender: "M"},
		"Zed d":         {Name: "Zed", Grade: "D"},
	}
	for in, want := range cases {
		if got := parseMemberLine(in); got != want {
			t.Errorf("parseMemberLine(%q) = %+v, want %+v", in, got, want)
		}
	}
}

func TestSplitNames(t *testing.T) {
	if got := splitNames("Alice Bob  Cara"); len(got) != 3 {
		t.Fatalf("got %v", got)
	}
	if got := splitNames("Mary Jane, Bob\nCara"); len(got) != 3 || got[0] != "Mary Jane" {
		t.Fatalf("got %v", got)
	}
}

func TestParseCommandAddIsOneEntry(t *testing.T) {
	cmd, ok := ParseCommand("add Mary Jane F")
	if !ok || !cmd.Single {
		t.Fatalf("got %+v ok=%v, want a single-entry in", cmd, ok)
	}
	if cmd, _ := ParseCommand("checkin Mary Jane"); cmd.Single {
		t.Fatal("checkin should split names")
	}
}

func TestParseInEntries(t *testing.T) {
	cases := map[string][]memberLine{
		"Andi Budi":             {{Name: "Andi"}, {Name: "Budi"}},
		"Eko M C":               {{Name: "Eko", Gender: "M", Grade: "C"}},
		"Andi Eko m c Fani F B": {{Name: "Andi"}, {Name: "Eko", Gender: "M", Grade: "C"}, {Name: "Fani", Gender: "F", Grade: "B"}},
		"Mary Jane F D, Budi":   {{Name: "Mary Jane", Gender: "F", Grade: "D"}, {Name: "Budi"}},
		"Andi\nBudi M":          {{Name: "Andi"}, {Name: "Budi", Gender: "M"}},
	}
	for in, want := range cases {
		got := parseInEntries(in)
		if len(got) != len(want) {
			t.Errorf("parseInEntries(%q) = %+v, want %+v", in, got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("parseInEntries(%q)[%d] = %+v, want %+v", in, i, got[i], want[i])
			}
		}
	}
}
