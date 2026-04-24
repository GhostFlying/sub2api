package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGroupCodexServiceTierMode(t *testing.T) {
	t.Parallel()

	require.Equal(t, CodexServiceTierModeFollowUpstream, (*Group)(nil).CodexServiceTierOverrideMode())
	require.Equal(t, CodexServiceTierModeForceFast, (&Group{
		CodexServiceTierMode: "force_fast",
	}).CodexServiceTierOverrideMode())
	require.Equal(t, CodexServiceTierModeDisallowFast, (&Group{
		CodexServiceTierMode: "disallow_fast",
	}).CodexServiceTierOverrideMode())
	require.Equal(t, CodexServiceTierModeFollowUpstream, (&Group{
		CodexServiceTierMode: "unexpected",
	}).CodexServiceTierOverrideMode())
}

func TestSanitizeGroupCodexServiceTierFields(t *testing.T) {
	t.Parallel()

	g := &Group{Platform: PlatformOpenAI, CodexServiceTierMode: "fast"}
	sanitizeGroupCodexServiceTierFields(g)
	require.Equal(t, CodexServiceTierModeForceFast, g.CodexServiceTierMode)

	g = &Group{Platform: PlatformAnthropic, CodexServiceTierMode: CodexServiceTierModeForceFast}
	sanitizeGroupCodexServiceTierFields(g)
	require.Equal(t, CodexServiceTierModeFollowUpstream, g.CodexServiceTierMode)
}

func TestCodexServiceTierOverrideEditMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		mode        string
		exists      bool
		raw         string
		rawIsString bool
		want        codexServiceTierEdit
	}{
		{
			name: "follow upstream leaves missing tier unchanged",
			mode: CodexServiceTierModeFollowUpstream,
			want: codexServiceTierEdit{},
		},
		{
			name:        "follow upstream leaves client fast unchanged",
			mode:        CodexServiceTierModeFollowUpstream,
			exists:      true,
			raw:         "fast",
			rawIsString: true,
			want:        codexServiceTierEdit{},
		},
		{
			name:        "follow upstream leaves client priority unchanged",
			mode:        CodexServiceTierModeFollowUpstream,
			exists:      true,
			raw:         "priority",
			rawIsString: true,
			want:        codexServiceTierEdit{},
		},
		{
			name:        "follow upstream leaves client flex unchanged",
			mode:        CodexServiceTierModeFollowUpstream,
			exists:      true,
			raw:         "flex",
			rawIsString: true,
			want:        codexServiceTierEdit{},
		},
		{
			name: "force fast sets missing tier to priority",
			mode: CodexServiceTierModeForceFast,
			want: codexServiceTierEdit{changed: true, value: "priority"},
		},
		{
			name:        "force fast converts client fast to priority",
			mode:        CodexServiceTierModeForceFast,
			exists:      true,
			raw:         "fast",
			rawIsString: true,
			want:        codexServiceTierEdit{changed: true, value: "priority"},
		},
		{
			name:        "force fast leaves normalized priority unchanged",
			mode:        CodexServiceTierModeForceFast,
			exists:      true,
			raw:         "priority",
			rawIsString: true,
			want:        codexServiceTierEdit{},
		},
		{
			name:        "force fast converts non-fast flex to priority",
			mode:        CodexServiceTierModeForceFast,
			exists:      true,
			raw:         "flex",
			rawIsString: true,
			want:        codexServiceTierEdit{changed: true, value: "priority"},
		},
		{
			name:        "force fast converts non-string tier to priority",
			mode:        CodexServiceTierModeForceFast,
			exists:      true,
			rawIsString: false,
			want:        codexServiceTierEdit{changed: true, value: "priority"},
		},
		{
			name: "disallow fast leaves missing tier unchanged",
			mode: CodexServiceTierModeDisallowFast,
			want: codexServiceTierEdit{},
		},
		{
			name:        "disallow fast removes client fast",
			mode:        CodexServiceTierModeDisallowFast,
			exists:      true,
			raw:         "fast",
			rawIsString: true,
			want:        codexServiceTierEdit{changed: true, delete: true},
		},
		{
			name:        "disallow fast removes normalized priority",
			mode:        CodexServiceTierModeDisallowFast,
			exists:      true,
			raw:         "priority",
			rawIsString: true,
			want:        codexServiceTierEdit{changed: true, delete: true},
		},
		{
			name:        "disallow fast removes case-insensitive fast",
			mode:        CodexServiceTierModeDisallowFast,
			exists:      true,
			raw:         " FAST ",
			rawIsString: true,
			want:        codexServiceTierEdit{changed: true, delete: true},
		},
		{
			name:        "disallow fast leaves normalized flex unchanged",
			mode:        CodexServiceTierModeDisallowFast,
			exists:      true,
			raw:         "flex",
			rawIsString: true,
			want:        codexServiceTierEdit{},
		},
		{
			name:        "disallow fast normalizes non-fast flex",
			mode:        CodexServiceTierModeDisallowFast,
			exists:      true,
			raw:         " flex ",
			rawIsString: true,
			want:        codexServiceTierEdit{changed: true, value: "flex"},
		},
		{
			name:        "disallow fast removes default tier",
			mode:        CodexServiceTierModeDisallowFast,
			exists:      true,
			raw:         "default",
			rawIsString: true,
			want:        codexServiceTierEdit{changed: true, delete: true},
		},
		{
			name:        "disallow fast removes unknown string tier",
			mode:        CodexServiceTierModeDisallowFast,
			exists:      true,
			raw:         "turbo",
			rawIsString: true,
			want:        codexServiceTierEdit{changed: true, delete: true},
		},
		{
			name:        "disallow fast removes non-string tier",
			mode:        CodexServiceTierModeDisallowFast,
			exists:      true,
			rawIsString: false,
			want:        codexServiceTierEdit{changed: true, delete: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := codexServiceTierOverrideEdit(tt.mode, tt.exists, tt.raw, tt.rawIsString)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestApplyCodexServiceTierOverrideToRequestMap(t *testing.T) {
	t.Parallel()

	reqBody := map[string]any{"model": "gpt-5.3-codex"}
	require.True(t, applyCodexServiceTierOverrideToRequestMap(reqBody, CodexServiceTierModeForceFast))
	require.Equal(t, "priority", reqBody["service_tier"])

	reqBody = map[string]any{"service_tier": "fast"}
	require.True(t, applyCodexServiceTierOverrideToRequestMap(reqBody, CodexServiceTierModeDisallowFast))
	require.NotContains(t, reqBody, "service_tier")

	reqBody = map[string]any{"service_tier": "priority"}
	require.True(t, applyCodexServiceTierOverrideToRequestMap(reqBody, CodexServiceTierModeDisallowFast))
	require.NotContains(t, reqBody, "service_tier")

	reqBody = map[string]any{"service_tier": " flex "}
	require.True(t, applyCodexServiceTierOverrideToRequestMap(reqBody, CodexServiceTierModeDisallowFast))
	require.Equal(t, "flex", reqBody["service_tier"])

	reqBody = map[string]any{"service_tier": "fast"}
	require.False(t, applyCodexServiceTierOverrideToRequestMap(reqBody, CodexServiceTierModeFollowUpstream))
	require.Equal(t, "fast", reqBody["service_tier"])

	reqBody = map[string]any{"type": "session.update"}
	require.False(t, applyCodexServiceTierOverrideToRequestMap(reqBody, CodexServiceTierModeForceFast))
	require.NotContains(t, reqBody, "service_tier")
}

func TestApplyCodexServiceTierOverrideToJSONBody(t *testing.T) {
	t.Parallel()

	body, changed, err := applyCodexServiceTierOverrideToJSONBody([]byte(`{"model":"gpt-5.3-codex"}`), CodexServiceTierModeForceFast)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "priority", gjson.GetBytes(body, "service_tier").String())

	body, changed, err = applyCodexServiceTierOverrideToJSONBody([]byte(`{"service_tier":"fast"}`), CodexServiceTierModeDisallowFast)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(body, "service_tier").Exists())

	body, changed, err = applyCodexServiceTierOverrideToJSONBody([]byte(`{"service_tier":" flex "}`), CodexServiceTierModeDisallowFast)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "flex", gjson.GetBytes(body, "service_tier").String())

	body, changed, err = applyCodexServiceTierOverrideToJSONBody([]byte(`{"service_tier":"fast"}`), CodexServiceTierModeFollowUpstream)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, "fast", gjson.GetBytes(body, "service_tier").String())

	body, changed, err = applyCodexServiceTierOverrideToJSONBody([]byte(`{"type":"session.update"}`), CodexServiceTierModeForceFast)
	require.NoError(t, err)
	require.False(t, changed)
	require.False(t, gjson.GetBytes(body, "service_tier").Exists())
}
