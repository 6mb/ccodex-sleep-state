// Package codexconfig makes a small, reversible edit to Codex's TOML.
// It never reads auth.json and never rewrites unrelated TOML tables.
package codexconfig

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gylive/ccodex-sleep-state/internal/settings"
	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

const provider = "ccodex-sleep-state"

type edit struct {
	start, end int
	text       string
}

func Patch(original []byte, baseURL string) ([]byte, error) {
	var document map[string]any
	if toml.Unmarshal(original, &document) != nil {
		return nil, errors.New("Codex config is not valid TOML; left unchanged")
	}
	if providers, ok := document["model_providers"].(map[string]any); ok {
		if _, exists := providers[provider]; exists {
			return nil, errors.New("provider name already exists; restore the previous service transaction first")
		}
	}
	replacements := map[string]string{"model": strconv.Quote(settings.Model), "model_provider": strconv.Quote(provider), "openai_base_url": strconv.Quote(baseURL)}
	var edits []edit
	var parser unstable.Parser
	parser.Reset(original)
	root := true
	for parser.NextExpression() {
		node := parser.Expression()
		if node.Kind == unstable.Table || node.Kind == unstable.ArrayTable {
			root = false
			continue
		}
		if !root || node.Kind != unstable.KeyValue {
			continue
		}
		keys := node.Key()
		var parts []string
		for keys.Next() {
			parts = append(parts, string(keys.Node().Data))
		}
		if len(parts) != 1 {
			continue
		}
		value, ok := replacements[parts[0]]
		if !ok {
			continue
		}
		raw := node.Value().Raw
		if raw.Length == 0 {
			return nil, errors.New("unsupported root setting; left unchanged")
		}
		edits = append(edits, edit{int(raw.Offset), int(raw.Offset + raw.Length), value})
		delete(replacements, parts[0])
	}
	if parser.Error() != nil {
		return nil, errors.New("cannot parse Codex settings; left unchanged")
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	updated := bytes.Clone(original)
	for _, e := range edits {
		updated = append(append(append([]byte{}, updated[:e.start]...), []byte(e.text)...), updated[e.end:]...)
	}
	var prefix strings.Builder
	for _, key := range []string{"model", "model_provider", "openai_base_url"} {
		if value, ok := replacements[key]; ok {
			fmt.Fprintf(&prefix, "%s = %s\n", key, value)
		}
	}
	updated = append([]byte(prefix.String()), updated...)
	block := fmt.Sprintf(`

# Managed while ccodex-sleep-state is running. Use its restore command after a crash.
[model_providers.%s]
name = "Sleep State (local)"
base_url = %s
wire_api = "responses"
requires_openai_auth = true
supports_websockets = false
request_max_retries = 0
stream_max_retries = 0
`, provider, strconv.Quote(baseURL))
	updated = append(updated, []byte(block)...)
	if toml.Unmarshal(updated, &document) != nil {
		return nil, errors.New("managed provider conflicts with existing TOML; left unchanged")
	}
	return updated, nil
}
