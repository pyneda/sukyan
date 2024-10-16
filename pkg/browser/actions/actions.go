package actions

import (
	"os"

	"gopkg.in/yaml.v3"
)

type ActionType string
type WaitCondition string
type AssertCondition string
type ScrollPosition string

const (
	ActionNavigate   ActionType = "navigate"
	ActionClick      ActionType = "click"
	ActionFill       ActionType = "fill"
	ActionWait       ActionType = "wait"
	ActionAssert     ActionType = "assert"
	ActionScroll     ActionType = "scroll"
	ActionScreenshot ActionType = "screenshot"
	ActionSleep      ActionType = "sleep"
	ActionEvaluate   ActionType = "evaluate"

	WaitVisible WaitCondition = "visible"
	WaitHidden  WaitCondition = "hidden"
	WaitEnabled WaitCondition = "enabled"
	WaitLoad    WaitCondition = "load"

	AssertContains AssertCondition = "contains"
	AssertEquals   AssertCondition = "equals"
	AssertVisible  AssertCondition = "visible"
	AssertHidden   AssertCondition = "hidden"

	ScrollTop    ScrollPosition = "top"
	ScrollBottom ScrollPosition = "bottom"
)

type Action struct {
	Type       ActionType      `yaml:"type" json:"type" validate:"required,oneof=navigate click fill wait assert scroll screenshot sleep evaluate"`
	Selector   string          `yaml:"selector,omitempty" json:"selector,omitempty" validate:"required_if=Type click,required_if=Type fill"`
	Value      string          `yaml:"value,omitempty" json:"value,omitempty" validate:"required_if=Type fill"`
	URL        string          `yaml:"url,omitempty" json:"url,omitempty" validate:"required_if=Type navigate,omitempty,url"`
	For        WaitCondition   `yaml:"for,omitempty" json:"for,omitempty" validate:"omitempty,oneof=visible hidden enabled load"`
	Condition  AssertCondition `yaml:"condition,omitempty" json:"condition,omitempty" validate:"required_if=Type assert,omitempty,oneof=contains equals visible hidden"`
	Position   ScrollPosition  `yaml:"position,omitempty" json:"position,omitempty" validate:"required_if=Type scroll,omitempty,oneof=top bottom"`
	File       string          `yaml:"file,omitempty" json:"file,omitempty" validate:"required_if=Type screenshot,omitempty"`
	Duration   int             `yaml:"duration,omitempty" json:"duration,omitempty" validate:"required_if=Type sleep,omitempty,gt=0"`
	Expression string          `yaml:"expression,omitempty" json:"expression,omitempty" validate:"required_if=Type evaluate,omitempty"`
}

type BrowserActions struct {
	Title   string   `yaml:"title" json:"title" validate:"required,min=1"`
	Actions []Action `yaml:"actions" json:"actions" validate:"required,min=1,dive"`
}

func LoadBrowserActions(path string) (BrowserActions, error) {
	var config BrowserActions
	data, err := os.ReadFile(path)
	if err != nil {
		return config, err
	}
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return config, err
	}

	return config, nil
}
