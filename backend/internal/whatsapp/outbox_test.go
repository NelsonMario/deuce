package whatsapp

import (
	"strings"
	"testing"
)

func TestFormatAnnouncementIncludesPublicMatchDetails(t *testing.T) {
	message := formatAnnouncement(Announcement{
		CourtName: "Court 2", Format: "MIXED_DOUBLES",
		TeamA: []string{"Alice", "Bob"}, TeamB: []string{"Cara", "Dan"},
	})
	for _, want := range []string{"Court 2", "Alice / Bob", "Cara / Dan", "MIXED_DOUBLES"} {
		if !strings.Contains(message, want) {
			t.Errorf("message %q does not include %q", message, want)
		}
	}
}

func TestWhatsmeowDatabaseURLUsesDedicatedSchema(t *testing.T) {
	got, err := whatsmeowDatabaseURL("postgres://user:pass@localhost:5432/deuce?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "search_path=whatsmeow%2Cpublic") {
		t.Fatalf("expected dedicated search path, got %s", got)
	}
}
