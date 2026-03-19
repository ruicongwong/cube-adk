package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"cube-adk/pkg/option"
	"cube-adk/pkg/protocol"
)

// DuckDuckGoTool searches the web using the DuckDuckGo Instant Answer API.
type DuckDuckGoTool struct {
	Client *http.Client
}

func NewDuckDuckGoTool() *DuckDuckGoTool {
	return &DuckDuckGoTool{Client: &http.Client{}}
}

func (d *DuckDuckGoTool) Identity() string { return "ddg_search" }
func (d *DuckDuckGoTool) Brief() string {
	return "Search the web using DuckDuckGo. Input: JSON {\"query\": \"search terms\"}"
}

func (d *DuckDuckGoTool) Spec() protocol.ToolSpec {
	return protocol.ToolSpec{
		Name: "ddg_search",
		Desc: d.Brief(),
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "search query"},
			},
			"required": []string{"query"},
		},
	}
}

func (d *DuckDuckGoTool) Run(ctx context.Context, call protocol.ToolCall, opts ...option.ToolOption) (protocol.ToolResult, error) {
	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(call.Args), &args); err != nil {
		return protocol.NewErrorResult(call.ID, fmt.Errorf("ddg: parse args: %w", err)), nil
	}
	if args.Query == "" {
		return protocol.NewErrorResult(call.ID, fmt.Errorf("ddg: empty query")), nil
	}

	apiURL := "https://api.duckduckgo.com/?q=" + url.QueryEscape(args.Query) + "&format=json&no_html=1&skip_disambig=1"

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return protocol.NewErrorResult(call.ID, err), nil
	}
	req.Header.Set("User-Agent", "cube-adk/1.0")

	client := d.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return protocol.NewErrorResult(call.ID, fmt.Errorf("ddg: http: %w", err)), nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return protocol.NewErrorResult(call.ID, fmt.Errorf("ddg: read body: %w", err)), nil
	}

	output, err := formatDDGResponse(body, args.Query)
	if err != nil {
		return protocol.NewErrorResult(call.ID, err), nil
	}
	return protocol.NewTextResult(call.ID, output), nil
}

type ddgResponse struct {
	Abstract       string     `json:"Abstract"`
	AbstractText   string     `json:"AbstractText"`
	AbstractSource string     `json:"AbstractSource"`
	AbstractURL    string     `json:"AbstractURL"`
	Heading        string     `json:"Heading"`
	Answer         string     `json:"Answer"`
	Definition     string     `json:"Definition"`
	RelatedTopics  []ddgTopic `json:"RelatedTopics"`
}

type ddgTopic struct {
	Text     string `json:"Text"`
	FirstURL string `json:"FirstURL"`
}

func formatDDGResponse(data []byte, query string) (string, error) {
	var resp ddgResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return string(data), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for: %s\n\n", query))

	if resp.Answer != "" {
		sb.WriteString("Answer: " + resp.Answer + "\n\n")
	}
	if resp.AbstractText != "" {
		sb.WriteString("Summary: " + resp.AbstractText + "\n")
		if resp.AbstractSource != "" {
			sb.WriteString("Source: " + resp.AbstractSource + " — " + resp.AbstractURL + "\n")
		}
		sb.WriteString("\n")
	}
	if resp.Definition != "" {
		sb.WriteString("Definition: " + resp.Definition + "\n\n")
	}

	if len(resp.RelatedTopics) > 0 {
		sb.WriteString("Related:\n")
		limit := 5
		if len(resp.RelatedTopics) < limit {
			limit = len(resp.RelatedTopics)
		}
		for i := 0; i < limit; i++ {
			t := resp.RelatedTopics[i]
			if t.Text != "" {
				sb.WriteString(fmt.Sprintf("- %s\n  %s\n", t.Text, t.FirstURL))
			}
		}
	}

	result := sb.String()
	if strings.TrimSpace(result) == fmt.Sprintf("Search results for: %s", query) {
		return fmt.Sprintf("No instant answer found for: %s. Try a more specific query.", query), nil
	}
	return result, nil
}
