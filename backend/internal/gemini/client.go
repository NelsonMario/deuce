package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	ErrAPIKeyMissing     = errors.New("gemini api key is missing")
	ErrRateLimitExceeded = errors.New("gemini rate limit exceeded")
)

// RateLimiter enforces a sliding window rate limit (requests per minute).
type RateLimiter struct {
	mu         sync.Mutex
	maxRPM     int
	timestamps []time.Time
}

func NewRateLimiter(maxRPM int) *RateLimiter {
	return &RateLimiter{maxRPM: maxRPM}
}

func (r *RateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.maxRPM <= 0 {
		return false
	}
	now := time.Now()
	cutoff := now.Add(-1 * time.Minute)
	valid := r.timestamps[:0]
	for _, t := range r.timestamps {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	r.timestamps = valid
	if len(r.timestamps) >= r.maxRPM {
		return false
	}
	r.timestamps = append(r.timestamps, now)
	return true
}

// CourtContext represents one court's current state for the LLM.
type CourtContext struct {
	Name   string
	Number int
	Status string // "playing", "available", etc.
	TeamA  []string
	TeamB  []string
}

// InterpretInput carries user input and live badminton session context.
type InterpretInput struct {
	Prompt        string
	SessionActive bool
	Courts        []CourtContext
	Waiting       []string
	ClubMembers   []string
}

// InterpretOutput is the structured response from Gemini.
type InterpretOutput struct {
	Commands []string `json:"commands"`
	Reply    string   `json:"reply"`
}

type Client struct {
	apiKey      string
	model       string
	baseURL     string
	httpClient  *http.Client
	rateLimiter *RateLimiter
}

func NewClient(apiKey, model string, maxRPM int) *Client {
	if model == "" {
		model = "gemini-2.5-flash"
	}
	if maxRPM <= 0 {
		maxRPM = 10
	}
	return &Client{
		apiKey:      strings.TrimSpace(apiKey),
		model:       model,
		baseURL:     "https://generativelanguage.googleapis.com",
		httpClient:  &http.Client{Timeout: 20 * time.Second},
		rateLimiter: NewRateLimiter(maxRPM),
	}
}

func (c *Client) SetBaseURL(url string) {
	c.baseURL = strings.TrimRight(url, "/")
}

func (c *Client) SetHTTPClient(client *http.Client) {
	c.httpClient = client
}

func (c *Client) IsConfigured() bool {
	return c.apiKey != ""
}

const systemPrompt = `You are the AI assistant for Deuce, a WhatsApp bot for casual badminton sessions.
Your job is to interpret natural language messages from the badminton host and convert them into one or more concrete Deuce bot commands, or provide a helpful reply/clarification.

Available Deuce bot commands:
1. "start" - Start the badminton session.
2. "stop" - End the badminton session and get the recap.
3. "status" - Show courts with lineups, waiting queue, and recent match results.
4. "courts <n>" - Set the total number of courts (e.g. "courts 2").
5. "in <names>" - Check in players into the waiting queue.
   - For registered members: "in Alice, Bob"
   - For new members with gender (M/F) and starting grade (A-E): "in Zed M C"
6. "out <name>" - Remove a player from the queue or court.
7. "next" - Automatically fill the next free court with waiting players.
8. "done <court> <scoreA>-<scoreB>" - Record the finished score on a court (e.g. "done 1 21-15").
   - Team A is listed first on the court. Winning team has the higher score.
9. "cancel" - Cancel the latest unfinished match; players return to queue.
10. "auto" - Enable automatic court filling.
11. "manual" - Switch to manual court filling.
12. "suggest <4 names>" - Suggest the most balanced split of four players.
13. "assign <court> <P1> <P2> vs <P3> <P4>" - Put an exact lineup on a court.
14. "member" - List club members or show a member's info ("member <name>").
15. "grade <name> [A-E]" - Show or reset a member's grade and rating ("grade Bob A").

Important Rules:
1. Fuzzy and Typo Name Matching:
   - Hosts often mistype names or use nicknames (e.g. "nels" or "nelsie" when the member is "Nelson").
   - Always check the registered club members and current court/queue players provided in context.
   - If a name is a clear typo, shorthand, or variant of an existing member, use the member's exact registered name in the command.
2. Court & Lineup Resolution:
   - If the host says "Alice and Bob won 21-15" without giving a court number, look at the active courts in context.
   - Determine which court Alice and Bob are on. If Team A is Alice & Bob, then scoreA is 21 and scoreB is 15. Produce "done <court> 21-15".
3. Ambiguity:
   - If a name is ambiguous (e.g. "Nel" when both "Nelson" and "Nellie" exist in the club), DO NOT guess. Return commands as empty [] and ask for clarification in "reply".
4. Multiple Actions:
   - If the host asks for multiple actions at once (e.g. "start session, 2 courts, and check in Alice and Bob"), return them sequentially: ["start", "courts 2", "in Alice Bob"].
5. Non-command or Questions:
   - If the host is asking a question or chatting, set commands to [] and answer helpfully in "reply".
6. Language:
   - Understand English, Indonesian (e.g. "lapangan 1 beres 21-18", "masukin andi sama budi"), or any other language.`

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiSchema struct {
	Type        string                  `json:"type"`
	Properties  map[string]geminiSchema `json:"properties,omitempty"`
	Items       *geminiSchema           `json:"items,omitempty"`
	Required    []string                `json:"required,omitempty"`
	Description string                  `json:"description,omitempty"`
}

type geminiGenerationConfig struct {
	Temperature      float64       `json:"temperature"`
	ResponseMimeType string        `json:"responseMimeType"`
	ResponseSchema   *geminiSchema `json:"responseSchema,omitempty"`
}

type geminiRequest struct {
	SystemInstruction *geminiContent         `json:"systemInstruction,omitempty"`
	Contents          []geminiContent        `json:"contents"`
	GenerationConfig  geminiGenerationConfig `json:"generationConfig"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

func (c *Client) Interpret(ctx context.Context, input InterpretInput) (*InterpretOutput, error) {
	if !c.IsConfigured() {
		return nil, ErrAPIKeyMissing
	}
	if !c.rateLimiter.Allow() {
		return nil, ErrRateLimitExceeded
	}

	contextMsg := buildContextMessage(input)

	reqPayload := geminiRequest{
		SystemInstruction: &geminiContent{
			Parts: []geminiPart{{Text: systemPrompt}},
		},
		Contents: []geminiContent{
			{
				Role: "user",
				Parts: []geminiPart{
					{Text: fmt.Sprintf("Current Badminton Session Context:\n%s\n\nHost Message:\n%s", contextMsg, input.Prompt)},
				},
			},
		},
		GenerationConfig: geminiGenerationConfig{
			Temperature:      0.1,
			ResponseMimeType: "application/json",
			ResponseSchema: &geminiSchema{
				Type: "OBJECT",
				Properties: map[string]geminiSchema{
					"commands": {
						Type:        "ARRAY",
						Description: "List of Deuce bot commands to run in order, e.g. ['done 1 21-15'] or ['in Alice Bob']. Empty if no action is needed or input is ambiguous.",
						Items: &geminiSchema{
							Type: "STRING",
						},
					},
					"reply": {
						Type:        "STRING",
						Description: "Explanation of the action taken, answer to host question, or clarification request if ambiguous.",
					},
				},
				Required: []string{"commands", "reply"},
			},
		},
	}

	bodyBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal gemini request: %w", err)
	}

	apiURL := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", c.baseURL, c.model, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create gemini request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute gemini request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read gemini response: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, ErrRateLimitExceeded
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini api error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var gResp geminiResponse
	if err := json.Unmarshal(respBytes, &gResp); err != nil {
		return nil, fmt.Errorf("unmarshal gemini response: %w", err)
	}
	if gResp.Error != nil {
		return nil, fmt.Errorf("gemini error: %s (status %s)", gResp.Error.Message, gResp.Error.Status)
	}
	if len(gResp.Candidates) == 0 || len(gResp.Candidates[0].Content.Parts) == 0 {
		return nil, errors.New("empty response from gemini")
	}

	responseText := gResp.Candidates[0].Content.Parts[0].Text
	var out InterpretOutput
	if err := json.Unmarshal([]byte(responseText), &out); err != nil {
		return nil, fmt.Errorf("unmarshal structured output %q: %w", responseText, err)
	}

	return &out, nil
}

func buildContextMessage(in InterpretInput) string {
	var sb strings.Builder
	if in.SessionActive {
		sb.WriteString("Session is currently ACTIVE.\n")
	} else {
		sb.WriteString("No session is active.\n")
	}

	if len(in.Courts) > 0 {
		sb.WriteString("Courts:\n")
		for _, c := range in.Courts {
			cName := c.Name
			if cName == "" && c.Number > 0 {
				cName = fmt.Sprintf("Court %d", c.Number)
			}
			if cName == "" {
				cName = "Court"
			}
			if len(c.TeamA) > 0 && len(c.TeamB) > 0 {
				sb.WriteString(fmt.Sprintf("• %s [%s]: %s vs %s\n",
					cName, c.Status, strings.Join(c.TeamA, " & "), strings.Join(c.TeamB, " & ")))
			} else {
				sb.WriteString(fmt.Sprintf("• %s [%s]: free\n", cName, c.Status))
			}
		}
	} else {
		sb.WriteString("Courts: none configured\n")
	}

	if len(in.Waiting) > 0 {
		sb.WriteString("Waiting Queue: " + strings.Join(in.Waiting, ", ") + "\n")
	} else {
		sb.WriteString("Waiting Queue: empty\n")
	}

	if len(in.ClubMembers) > 0 {
		sb.WriteString("Registered Club Members: " + strings.Join(in.ClubMembers, ", ") + "\n")
	} else {
		sb.WriteString("Registered Club Members: none\n")
	}

	return sb.String()
}
