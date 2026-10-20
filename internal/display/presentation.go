package display

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"strings"
	"time"
)

// Theme describes the data and dimensions requested by a declarative style.
type Theme struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	DataLayout string `json:"dataLayout"`
}

func ReadThemes(files fs.FS) (map[string]Theme, error) {
	paths, err := fs.Glob(files, "themes/*/theme.json")
	if err != nil {
		return nil, err
	}
	services, err := fs.Glob(files, "themes/services/*/theme.json")
	if err != nil {
		return nil, err
	}
	paths = append(paths, services...)
	themes := map[string]Theme{}
	for _, path := range paths {
		contents, err := fs.ReadFile(files, path)
		if err != nil {
			return nil, err
		}
		var theme Theme
		if err := json.Unmarshal(contents, &theme); err != nil {
			return nil, err
		}
		if theme.ID == "" || themes[theme.ID].ID != "" {
			return nil, fmt.Errorf("invalid or duplicate style in %s", path)
		}
		themes[theme.ID] = theme
	}
	return themes, nil
}

type ServiceContent struct {
	Enabled bool   `json:"enabled"`
	Content string `json:"content"`
}

func (c *ServiceContent) UnmarshalJSON(data []byte) error {
	var entry struct {
		Enabled *bool   `json:"enabled"`
		Content *string `json:"content"`
	}
	if err := json.Unmarshal(data, &entry); err != nil {
		return err
	}
	if entry.Enabled == nil || entry.Content == nil {
		return fmt.Errorf("service settings require enabled and content")
	}
	c.Enabled, c.Content = *entry.Enabled, *entry.Content
	return nil
}

// Presentation is the runtime contract; its disk representation is agreed separately.
type Presentation struct {
	Reverse180    bool                                 `json:"reverse180"`
	BlankOnUpdate bool                                 `json:"blankOnUpdate"`
	SkipFull5h    bool                                 `json:"skipFull5h"`
	Services      map[string]map[string]ServiceContent `json:"services"`
}

func ClonePresentation(value *Presentation) *Presentation {
	if value == nil {
		return nil
	}
	clone := *value
	clone.Services = make(map[string]map[string]ServiceContent, len(value.Services))
	for source, services := range value.Services {
		clone.Services[source] = maps.Clone(services)
	}
	return &clone
}

func ValidatePresentation(value *Presentation) error {
	for source, services := range value.Services {
		if source != "Local" && source != "Hub" {
			return fmt.Errorf("choose Local or Hub")
		}
		for id, content := range services {
			if id == "" || (content.Content != "Both" && content.Content != "Limits" && content.Content != "Tokens") {
				return fmt.Errorf("choose Both, Limits or Tokens")
			}
		}
	}
	return nil
}

func (p Presentation) content(source, id string) ServiceContent {
	if content, ok := p.Services[source][id]; ok {
		return content
	}
	return ServiceContent{Enabled: true, Content: "Both"}
}

type Configuration struct {
	TargetID        string
	TargetConnected bool
	CycleEvery      time.Duration
	Theme           Theme
	Presentation    Presentation
	Source          string
	Cycle           bool
	CycleSeconds    int
	Generation      uint64
}

func (t Theme) ServiceLayout() bool { return strings.HasPrefix(t.DataLayout, "service-") }
