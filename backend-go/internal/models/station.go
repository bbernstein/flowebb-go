package models

type Source string

const (
	SourceNOAA Source = "NOAA"
	SourceUKHO Source = "UKHO"
	SourceCHS  Source = "CHS"
)

type Station struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	State          *string  `json:"state,omitempty"`
	Region         *string  `json:"region,omitempty"`
	Distance       float64  `json:"distance"`
	Latitude       float64  `json:"latitude"`
	Longitude      float64  `json:"longitude"`
	Source         Source   `json:"source"`
	Capabilities   []string `json:"capabilities"`
	TimeZoneOffset int      `json:"timeZoneOffset"`
	Level          *string  `json:"level,omitempty"`
	StationType    *string  `json:"stationType,omitempty"`
}
