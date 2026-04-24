package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	CodexServiceTierModeFollowUpstream = "follow_upstream"
	CodexServiceTierModeForceFast      = "force_fast"
	CodexServiceTierModeDisallowFast   = "disallow_fast"
)

func normalizeCodexServiceTierMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "follow", "follow_upstream", "upstream":
		return CodexServiceTierModeFollowUpstream
	case "force_fast", "fast", "force_priority", "priority":
		return CodexServiceTierModeForceFast
	case "disallow_fast", "no_fast", "disable_fast", "default", "standard":
		return CodexServiceTierModeDisallowFast
	default:
		return CodexServiceTierModeFollowUpstream
	}
}

func (g *Group) CodexServiceTierOverrideMode() string {
	if g == nil {
		return CodexServiceTierModeFollowUpstream
	}
	return normalizeCodexServiceTierMode(g.CodexServiceTierMode)
}

func sanitizeGroupCodexServiceTierFields(g *Group) {
	if g == nil {
		return
	}
	if g.Platform != PlatformOpenAI {
		g.CodexServiceTierMode = CodexServiceTierModeFollowUpstream
		return
	}
	g.CodexServiceTierMode = g.CodexServiceTierOverrideMode()
}

func shouldApplyCodexServiceTierOverride(account *Account) bool {
	return account != nil && account.Platform == PlatformOpenAI && account.Type == AccountTypeOAuth
}

func (s *OpenAIGatewayService) resolveCodexServiceTierOverrideMode(ctx context.Context, c *gin.Context, account *Account) string {
	// Only Codex/OpenAI OAuth upstreams understand this policy. API-key accounts
	// and non-OpenAI platforms should behave exactly as the client requested.
	if !shouldApplyCodexServiceTierOverride(account) {
		return CodexServiceTierModeFollowUpstream
	}
	if group := getCodexServiceTierGroupFromContext(c); group != nil {
		return group.CodexServiceTierOverrideMode()
	}
	return CodexServiceTierModeFollowUpstream
}

func getCodexServiceTierGroupFromContext(c *gin.Context) *Group {
	if c == nil {
		return nil
	}
	// Auth middleware stores the API key on gin.Context; API keys are only the
	// lookup path, while the actual policy is read from apiKey.Group.
	if value, exists := c.Get("api_key"); exists {
		if apiKey, ok := value.(*APIKey); ok && apiKey != nil && apiKey.Group != nil {
			return apiKey.Group
		}
	}
	// Some internal paths attach the group directly to request context instead
	// of carrying the API key object through the handler stack.
	if c.Request != nil {
		if group, ok := c.Request.Context().Value(ctxkey.Group).(*Group); ok {
			return group
		}
	}
	return nil
}

// applyCodexServiceTierOverrideToRequestMap rewrites a decoded Responses
// request map, used by paths that already unmarshal or rebuild the request.
func applyCodexServiceTierOverrideToRequestMap(reqBody map[string]any, mode string) bool {
	if reqBody == nil {
		return false
	}
	if !shouldApplyCodexServiceTierOverrideForType(reqBody["type"]) {
		return false
	}
	raw, exists := reqBody["service_tier"]
	rawString, rawIsString := raw.(string)
	edit := codexServiceTierOverrideEdit(mode, exists, rawString, rawIsString)
	if !edit.changed {
		return false
	}
	if edit.delete {
		delete(reqBody, "service_tier")
		return true
	}
	reqBody["service_tier"] = edit.value
	return true
}

// applyCodexServiceTierOverrideToJSONBody rewrites a raw JSON request body,
// used by passthrough and WebSocket paths that should preserve the full payload.
func applyCodexServiceTierOverrideToJSONBody(body []byte, mode string) ([]byte, bool, error) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body, false, nil
	}
	if !shouldApplyCodexServiceTierOverrideForType(gjson.GetBytes(body, "type")) {
		return body, false, nil
	}
	raw := gjson.GetBytes(body, "service_tier")
	edit := codexServiceTierOverrideEdit(mode, raw.Exists(), raw.String(), raw.Type == gjson.String)
	if !edit.changed {
		return body, false, nil
	}
	if edit.delete {
		updated, err := sjson.DeleteBytes(body, "service_tier")
		if err != nil {
			return body, false, fmt.Errorf("apply codex service_tier disallow_fast: %w", err)
		}
		return updated, true, nil
	}
	updated, err := sjson.SetBytes(body, "service_tier", edit.value)
	if err != nil {
		return body, false, fmt.Errorf("apply codex service_tier override: %w", err)
	}
	return updated, true, nil
}

type codexServiceTierEdit struct {
	changed bool
	delete  bool
	value   string
}

func codexServiceTierOverrideEdit(mode string, exists bool, raw string, rawIsString bool) codexServiceTierEdit {
	// One edit planner feeds both map and raw-JSON callers, keeping force/disallow
	// semantics identical across normal Responses, compat, passthrough, and WS.
	switch normalizeCodexServiceTierMode(mode) {
	case CodexServiceTierModeForceFast:
		if rawIsString && raw == "priority" {
			return codexServiceTierEdit{}
		}
		return codexServiceTierEdit{changed: true, value: "priority"}
	case CodexServiceTierModeDisallowFast:
		if !exists {
			return codexServiceTierEdit{}
		}
		if rawIsString {
			normalized := normalizeOpenAIServiceTier(raw)
			if normalized != nil && *normalized == "flex" {
				// "flex" is not fast, so disallow_fast preserves it while still
				// normalizing whitespace/case to the upstream value.
				if raw == "flex" {
					return codexServiceTierEdit{}
				}
				return codexServiceTierEdit{changed: true, value: "flex"}
			}
		}
		return codexServiceTierEdit{changed: true, delete: true}
	default:
		return codexServiceTierEdit{}
	}
}

func shouldApplyCodexServiceTierOverrideForType(raw any) bool {
	// HTTP Responses bodies usually have no top-level "type"; WebSocket frames
	// do. Restrict WS rewriting to response.create so session.update and other
	// control messages are not polluted with request-only service_tier.
	switch v := raw.(type) {
	case nil:
		return true
	case string:
		eventType := strings.TrimSpace(v)
		return eventType == "" || eventType == "response.create"
	case gjson.Result:
		if !v.Exists() {
			return true
		}
		if v.Type != gjson.String {
			return false
		}
		eventType := strings.TrimSpace(v.String())
		return eventType == "" || eventType == "response.create"
	default:
		return false
	}
}
