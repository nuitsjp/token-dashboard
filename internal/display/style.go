package display

// Style is how Usage Limits are drawn.
type Style string

const (
	Gauges Style = "Gauges"
	Bars   Style = "Bars"
)

// ParseStyle returns Bars for "Bars" and Gauges for anything else, so an unset value means Gauges.
func ParseStyle(s string) Style {
	if s == string(Bars) {
		return Bars
	}
	return Gauges
}
