package model

import (
	"encoding/json"
	"fmt"
)

// FeatureAction is returned by poll to tell the client what to do.
type FeatureAction int

const (
	ActionDialogStep FeatureAction = iota
	ActionGenerate
)

var featureActionNames = map[FeatureAction]string{
	ActionDialogStep: "dialog_step",
	ActionGenerate:   "generate",
}

var featureActionValues = map[string]FeatureAction{}

func init() {
	for k, v := range featureActionNames {
		featureActionValues[v] = k
	}
}

func (a FeatureAction) String() string {
	name, ok := featureActionNames[a]
	if !ok {
		return fmt.Sprintf("unknown(%d)", int(a))
	}
	return name
}

func (a FeatureAction) MarshalJSON() ([]byte, error) {
	name, ok := featureActionNames[a]
	if !ok {
		return nil, fmt.Errorf("invalid FeatureAction %d", int(a))
	}
	return json.Marshal(name)
}

func (a *FeatureAction) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	v, ok := featureActionValues[str]
	if !ok {
		return fmt.Errorf("invalid FeatureAction %q", str)
	}
	*a = v
	return nil
}
