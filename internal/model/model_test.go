package model

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// --- FeatureStatus tests ---

func TestFeatureStatusString(t *testing.T) {
	cases := []struct {
		status FeatureStatus
		want   string
	}{
		{StatusDraft, "draft"},
		{StatusAwaitingClient, "awaiting_client"},
		{StatusAwaitingHuman, "awaiting_human"},
		{StatusFullySpecified, "fully_specified"},
		{StatusWaiting, "waiting"},
		{StatusReadyToGenerate, "ready_to_generate"},
		{StatusGenerating, "generating"},
		{StatusBeadsCreated, "beads_created"},
		{StatusDone, "done"},
		{StatusHalted, "halted"},
		{StatusAbandoned, "abandoned"},
	}
	for _, c := range cases {
		if got := c.status.String(); got != c.want {
			t.Errorf("FeatureStatus(%d).String() = %q, want %q", int(c.status), got, c.want)
		}
	}
}

func TestFeatureStatusUnknown(t *testing.T) {
	s := FeatureStatus(999)
	got := s.String()
	if !strings.HasPrefix(got, "unknown(") {
		t.Errorf("expected unknown prefix, got %q", got)
	}
}

func TestFeatureStatusMarshalJSON(t *testing.T) {
	data, err := json.Marshal(StatusDraft)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `"draft"` {
		t.Errorf("got %s, want \"draft\"", data)
	}
}

func TestFeatureStatusUnmarshalJSON(t *testing.T) {
	var s FeatureStatus
	if err := json.Unmarshal([]byte(`"ready_to_generate"`), &s); err != nil {
		t.Fatal(err)
	}
	if s != StatusReadyToGenerate {
		t.Errorf("got %v, want StatusReadyToGenerate", s)
	}
}

func TestFeatureStatusUnmarshalInvalid(t *testing.T) {
	var s FeatureStatus
	if err := json.Unmarshal([]byte(`"not_a_status"`), &s); err == nil {
		t.Error("expected error for invalid status, got nil")
	}
}

func TestFeatureStatusMarshalInvalid(t *testing.T) {
	s := FeatureStatus(999)
	if _, err := json.Marshal(s); err == nil {
		t.Error("expected error marshaling invalid FeatureStatus, got nil")
	}
}

func TestFeatureStatusRoundTrip(t *testing.T) {
	statuses := []FeatureStatus{
		StatusDraft, StatusAwaitingClient, StatusAwaitingHuman, StatusFullySpecified,
		StatusWaiting, StatusReadyToGenerate, StatusGenerating, StatusBeadsCreated,
		StatusDone, StatusHalted, StatusAbandoned,
	}
	for _, orig := range statuses {
		data, err := json.Marshal(orig)
		if err != nil {
			t.Fatalf("marshal %v: %v", orig, err)
		}
		var got FeatureStatus
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal %v: %v", orig, err)
		}
		if got != orig {
			t.Errorf("round-trip failed: got %v, want %v", got, orig)
		}
	}
}

// --- FeatureAction tests ---

func TestFeatureActionString(t *testing.T) {
	cases := []struct {
		action FeatureAction
		want   string
	}{
		{ActionDialogStep, "dialog_step"},
		{ActionGenerate, "generate"},
	}
	for _, c := range cases {
		if got := c.action.String(); got != c.want {
			t.Errorf("FeatureAction(%d).String() = %q, want %q", int(c.action), got, c.want)
		}
	}
}

func TestFeatureActionUnknown(t *testing.T) {
	a := FeatureAction(999)
	got := a.String()
	if !strings.HasPrefix(got, "unknown(") {
		t.Errorf("expected unknown prefix, got %q", got)
	}
}

func TestFeatureActionMarshalJSON(t *testing.T) {
	data, err := json.Marshal(ActionGenerate)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `"generate"` {
		t.Errorf("got %s, want \"generate\"", data)
	}
}

func TestFeatureActionUnmarshalJSON(t *testing.T) {
	var a FeatureAction
	if err := json.Unmarshal([]byte(`"dialog_step"`), &a); err != nil {
		t.Fatal(err)
	}
	if a != ActionDialogStep {
		t.Errorf("got %v, want ActionDialogStep", a)
	}
}

func TestFeatureActionUnmarshalInvalid(t *testing.T) {
	var a FeatureAction
	if err := json.Unmarshal([]byte(`"bad_action"`), &a); err == nil {
		t.Error("expected error for invalid action, got nil")
	}
}

func TestFeatureActionMarshalInvalid(t *testing.T) {
	a := FeatureAction(999)
	if _, err := json.Marshal(a); err == nil {
		t.Error("expected error marshaling invalid FeatureAction, got nil")
	}
}

func TestFeatureActionRoundTrip(t *testing.T) {
	for _, orig := range []FeatureAction{ActionDialogStep, ActionGenerate} {
		data, err := json.Marshal(orig)
		if err != nil {
			t.Fatalf("marshal %v: %v", orig, err)
		}
		var got FeatureAction
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal %v: %v", orig, err)
		}
		if got != orig {
			t.Errorf("round-trip failed: got %v, want %v", got, orig)
		}
	}
}

// --- ID generation tests ---

func TestGenerateFeatureIDFormat(t *testing.T) {
	id, err := GenerateFeatureID(func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, "ft-") {
		t.Errorf("id %q missing ft- prefix", id)
	}
	suffix := id[len("ft-"):]
	if len(suffix) < 4 || len(suffix) > 8 {
		t.Errorf("suffix length %d out of range [4,8]", len(suffix))
	}
	for _, c := range suffix {
		if !strings.ContainsRune(idChars, c) {
			t.Errorf("invalid character %q in id %q", c, id)
		}
	}
}

func TestGenerateFeatureIDCollisionEscalation(t *testing.T) {
	// Reject all 4-char IDs to force escalation to 5 chars
	calls := 0
	id, err := GenerateFeatureID(func(candidate string) bool {
		suffix := candidate[len("ft-"):]
		if len(suffix) == 4 {
			calls++
			return true // simulate collision
		}
		return false
	})
	if err != nil {
		t.Fatal(err)
	}
	suffix := id[len("ft-"):]
	if len(suffix) < 5 {
		t.Errorf("expected escalation beyond 4 chars, got suffix %q (len %d)", suffix, len(suffix))
	}
	if calls == 0 {
		t.Error("expected at least one collision at length 4")
	}
}

func TestGenerateFeatureIDUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id, err := GenerateFeatureID(func(s string) bool { return seen[s] })
		if err != nil {
			t.Fatal(err)
		}
		if seen[id] {
			t.Fatalf("duplicate ID generated: %q", id)
		}
		seen[id] = true
	}
}

// --- Struct JSON round-trip tests ---

func TestProjectRoundTrip(t *testing.T) {
	orig := Project{
		Name:      "my-project",
		Token:     "secret-token",
		CreatedAt: time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC),
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got Project
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != orig.Name || got.Token != orig.Token || !got.CreatedAt.Equal(orig.CreatedAt) {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, orig)
	}
}

func TestFeatureRoundTrip(t *testing.T) {
	orig := Feature{
		ID:               "ft-abcd",
		Project:          "proj1",
		Name:             "My Feature",
		Status:           StatusWaiting,
		CurrentIteration: 2,
		GenerateAfter:    "ft-xyz1",
		BeadIDs:          []string{"bd-0001", "bd-0002"},
		CreatedAt:        time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC),
		UpdatedAt:        time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC),
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got Feature
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != orig.ID || got.Project != orig.Project || got.Name != orig.Name ||
		got.Status != orig.Status || got.CurrentIteration != orig.CurrentIteration ||
		got.GenerateAfter != orig.GenerateAfter ||
		!got.CreatedAt.Equal(orig.CreatedAt) || !got.UpdatedAt.Equal(orig.UpdatedAt) {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, orig)
	}
	if len(got.BeadIDs) != len(orig.BeadIDs) {
		t.Fatalf("BeadIDs length mismatch: got %d, want %d", len(got.BeadIDs), len(orig.BeadIDs))
	}
	for i := range orig.BeadIDs {
		if got.BeadIDs[i] != orig.BeadIDs[i] {
			t.Errorf("BeadIDs[%d] = %q, want %q", i, got.BeadIDs[i], orig.BeadIDs[i])
		}
	}
}

func TestFeatureOmitEmptyFields(t *testing.T) {
	f := Feature{
		ID:      "ft-0001",
		Project: "p",
		Name:    "n",
		Status:  StatusDraft,
	}
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	// GenerateAfter and BeadIDs should be omitted when empty
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["generate_after"]; ok {
		t.Error("generate_after should be omitted when empty")
	}
	if _, ok := m["bead_ids"]; ok {
		t.Error("bead_ids should be omitted when empty")
	}
}

func TestDialogIterationRoundTrip(t *testing.T) {
	orig := DialogIteration{
		Round:     3,
		IsFinal:   true,
		CreatedAt: time.Date(2026, 3, 7, 9, 0, 0, 0, time.UTC),
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got DialogIteration
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Round != orig.Round || got.IsFinal != orig.IsFinal || !got.CreatedAt.Equal(orig.CreatedAt) {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, orig)
	}
}
