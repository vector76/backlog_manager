package model

import (
	"encoding/json"
	"fmt"
)

// FeatureStatus represents the lifecycle state of a feature.
type FeatureStatus int

const (
	StatusDraft FeatureStatus = iota
	StatusAwaitingClient
	StatusAwaitingHuman
	StatusFullySpecified
	StatusWaiting
	StatusReadyToGenerate
	StatusGenerating
	StatusBeadsCreated
	StatusDone
	StatusHalted
	StatusAbandoned
)

var featureStatusNames = map[FeatureStatus]string{
	StatusDraft:           "draft",
	StatusAwaitingClient:  "awaiting_client",
	StatusAwaitingHuman:   "awaiting_human",
	StatusFullySpecified:  "fully_specified",
	StatusWaiting:         "waiting",
	StatusReadyToGenerate: "ready_to_generate",
	StatusGenerating:      "generating",
	StatusBeadsCreated:    "beads_created",
	StatusDone:            "done",
	StatusHalted:          "halted",
	StatusAbandoned:       "abandoned",
}

var featureStatusValues = map[string]FeatureStatus{}

func init() {
	for k, v := range featureStatusNames {
		featureStatusValues[v] = k
	}
}

func (s FeatureStatus) String() string {
	name, ok := featureStatusNames[s]
	if !ok {
		return fmt.Sprintf("unknown(%d)", int(s))
	}
	return name
}

func (s FeatureStatus) MarshalJSON() ([]byte, error) {
	name, ok := featureStatusNames[s]
	if !ok {
		return nil, fmt.Errorf("invalid FeatureStatus %d", int(s))
	}
	return json.Marshal(name)
}

func (s *FeatureStatus) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	v, ok := featureStatusValues[str]
	if !ok {
		return fmt.Errorf("invalid FeatureStatus %q", str)
	}
	*s = v
	return nil
}
