package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestClient_ErrAPIKeyMissing(t *testing.T) {
	c := NewClient("", "gemini-2.5-flash", 10)
	_, err := c.Interpret(context.Background(), InterpretInput{Prompt: "hello"})
	if err != ErrAPIKeyMissing {
		t.Fatalf("expected ErrAPIKeyMissing, got %v", err)
	}
}

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(2)
	if !rl.Allow() {
		t.Fatal("expected 1st call to be allowed")
	}
	if !rl.Allow() {
		t.Fatal("expected 2nd call to be allowed")
	}
	if rl.Allow() {
		t.Fatal("expected 3rd call to be rejected (max 2)")
	}
}

func TestClient_Interpret_Success(t *testing.T) {
	expectedOut := InterpretOutput{
		Commands: []string{"done 1 21-15"},
		Reply:    "Court 1 match recorded: 21-15.",
	}

	outJSON, _ := json.Marshal(expectedOut)
	mockResp := geminiResponse{
		Candidates: []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		}{
			{
				Content: struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				}{
					Parts: []struct {
						Text string `json:"text"`
					}{
						{Text: string(outJSON)},
					},
				},
			},
		},
	}
	respBytes, _ := json.Marshal(mockResp)

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", req.Method)
			}
			if req.URL.Query().Get("key") != "test-key" {
				t.Errorf("expected key=test-key, got %s", req.URL.Query().Get("key"))
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(respBytes)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	c := NewClient("test-key", "gemini-2.5-flash", 10)
	c.SetHTTPClient(client)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res, err := c.Interpret(ctx, InterpretInput{
		Prompt:        "court 1 finished 21-15",
		SessionActive: true,
		Courts: []CourtContext{
			{Number: 1, Status: "playing", TeamA: []string{"Nelson", "Bob"}, TeamB: []string{"Cara", "Dan"}},
		},
		ClubMembers: []string{"Nelson", "Bob", "Cara", "Dan"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.Commands) != 1 || res.Commands[0] != "done 1 21-15" {
		t.Fatalf("unexpected commands: %+v", res.Commands)
	}
	if res.Reply != expectedOut.Reply {
		t.Fatalf("unexpected reply: %s", res.Reply)
	}
}

func TestClient_RateLimitExceeded(t *testing.T) {
	c := NewClient("test-key", "gemini-2.5-flash", 1)
	c.rateLimiter = NewRateLimiter(0) // immediately disallow

	_, err := c.Interpret(context.Background(), InterpretInput{Prompt: "hello"})
	if err != ErrRateLimitExceeded {
		t.Fatalf("expected ErrRateLimitExceeded, got %v", err)
	}
}
