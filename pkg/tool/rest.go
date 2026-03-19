package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"text/template"

	"cube-adk/pkg/option"
	"cube-adk/pkg/protocol"
)

var pathParamRe = regexp.MustCompile(`\{(\w+)\}`)
var tplFieldRe = regexp.MustCompile(`\{\{\.(\w+)\}\}`)

// RESTSpec declaratively describes an HTTP endpoint to be wrapped as a Tool.
type RESTSpec struct {
	Name        string
	Desc        string
	Method      string
	URL         string
	Headers     map[string]string
	QueryParams map[string]string
	BodyTpl     string
	ResultPath  string
}

// restTool wraps a RESTSpec into a component.Tool.
type restTool struct {
	spec   RESTSpec
	schema map[string]any
	tpl    *template.Template
	client *http.Client
}

// NewRESTTool creates a Tool from a RESTSpec declaration.
func NewRESTTool(spec RESTSpec) *restTool {
	return NewRESTToolWithClient(spec, &http.Client{})
}

// NewRESTToolWithClient creates a RESTTool with a custom http.Client.
func NewRESTToolWithClient(spec RESTSpec, client *http.Client) *restTool {
	t := &restTool{spec: spec, client: client}
	t.schema = t.buildSchema()
	if spec.BodyTpl != "" {
		t.tpl, _ = template.New("body").Parse(spec.BodyTpl)
	}
	return t
}

func (t *restTool) Identity() string { return t.spec.Name }
func (t *restTool) Brief() string    { return t.spec.Desc }

func (t *restTool) Spec() protocol.ToolSpec {
	return protocol.ToolSpec{Name: t.spec.Name, Desc: t.spec.Desc, Schema: t.schema}
}

func (t *restTool) buildSchema() map[string]any {
	props := map[string]any{}
	for _, m := range pathParamRe.FindAllStringSubmatch(t.spec.URL, -1) {
		props[m[1]] = map[string]any{"type": "string", "description": "path parameter: " + m[1]}
	}
	if t.spec.BodyTpl != "" {
		for _, m := range tplFieldRe.FindAllStringSubmatch(t.spec.BodyTpl, -1) {
			props[m[1]] = map[string]any{"type": "string", "description": "body field: " + m[1]}
		}
	}
	required := make([]string, 0, len(props))
	for k := range props {
		required = append(required, k)
	}
	return map[string]any{
		"type":       "object",
		"properties": props,
		"required":   required,
	}
}

func (t *restTool) Run(ctx context.Context, call protocol.ToolCall, opts ...option.ToolOption) (protocol.ToolResult, error) {
	var args map[string]string
	if err := json.Unmarshal([]byte(call.Args), &args); err != nil {
		return protocol.NewErrorResult(call.ID, fmt.Errorf("resttool: parse args: %w", err)), nil
	}

	url := t.spec.URL
	for _, m := range pathParamRe.FindAllStringSubmatch(url, -1) {
		val, ok := args[m[1]]
		if !ok {
			return protocol.NewErrorResult(call.ID, fmt.Errorf("resttool: missing path param %q", m[1])), nil
		}
		url = strings.Replace(url, m[0], val, 1)
	}

	if len(t.spec.QueryParams) > 0 {
		sep := "?"
		if strings.Contains(url, "?") {
			sep = "&"
		}
		for k, v := range t.spec.QueryParams {
			url += sep + k + "=" + v
			sep = "&"
		}
	}

	var bodyReader io.Reader
	if t.tpl != nil {
		var buf bytes.Buffer
		if err := t.tpl.Execute(&buf, args); err != nil {
			return protocol.NewErrorResult(call.ID, fmt.Errorf("resttool: render body: %w", err)), nil
		}
		bodyReader = &buf
	}

	req, err := http.NewRequestWithContext(ctx, t.spec.Method, url, bodyReader)
	if err != nil {
		return protocol.NewErrorResult(call.ID, err), nil
	}
	for k, v := range t.spec.Headers {
		req.Header.Set(k, v)
	}
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return protocol.NewErrorResult(call.ID, fmt.Errorf("resttool: http: %w", err)), nil
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return protocol.NewErrorResult(call.ID, fmt.Errorf("resttool: read: %w", err)), nil
	}

	output := string(data)
	if t.spec.ResultPath != "" {
		output, _ = extractPath(data, t.spec.ResultPath)
	}
	return protocol.NewTextResult(call.ID, output), nil
}

func extractPath(data []byte, path string) (string, error) {
	var obj any
	if err := json.Unmarshal(data, &obj); err != nil {
		return string(data), nil
	}
	parts := strings.Split(path, ".")
	cur := obj
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return string(data), nil
		}
		cur, ok = m[p]
		if !ok {
			return string(data), nil
		}
	}
	out, err := json.Marshal(cur)
	if err != nil {
		return fmt.Sprintf("%v", cur), nil
	}
	return string(out), nil
}
