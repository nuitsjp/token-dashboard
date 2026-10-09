package display

// tokscale 4.17.0 reports usage provider labels and graph client IDs separately.
// Only these verified tool pairs are aliases in the Local compact display.
var localServiceIDs = [...]struct{ provider, client string }{
	{"Claude", "claude"},
	{"Codex", "codex"},
	{"Amp", "amp"},
	{"Antigravity", "antigravity"},
	{"Copilot", "copilot"},
	{"Grok Build", "grok"},
	{"Kimi", "kimi"},
	{"Warp/Oz", "warp"},
}

func serviceID(source, reported string) string {
	if source == "Local" {
		for _, id := range localServiceIDs {
			if reported == id.client {
				return id.provider
			}
		}
	}
	return reported
}

func tokenServiceID(source, service string) string {
	if source == "Local" {
		for _, id := range localServiceIDs {
			if service == id.provider {
				return id.client
			}
		}
	}
	return service
}

func serviceContent(selection *serviceSelection, source, service string) ServiceContent {
	key := service
	if selection != nil {
		if _, exists := selection.content[key]; !exists {
			key = tokenServiceID(source, service)
		}
	}
	content := contentOf(selection, key)
	content.Provider = service
	return content
}
