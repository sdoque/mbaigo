package forms

// MessageLevel indicates the importance or criticality of a message.
type MessageLevel int

// Mimics the levels from the "slog" package
const (
	LevelDebug MessageLevel = -4
	LevelInfo  MessageLevel = 0
	LevelWarn  MessageLevel = 4
	LevelError MessageLevel = 8
)

// A SystemMessage is a log message sent from a system to one or many messengers.
// The receiving messengers will note the message's time of arrival.
type SystemMessage_v1 struct {
	Level   MessageLevel `json:"level"`
	Body    string       `json:"body"`
	System  string       `json:"system"`
	Version string       `json:"version"`
}

func NewSystemMessage_v1(l MessageLevel, b string, s string) Form {
	return &SystemMessage_v1{
		Level:   l,
		Body:    b,
		System:  s,
		Version: "SystemMessage_v1",
	}
}

// NewForm resets the form and defaults to LevelInfo.
//
// Note: Unnecessary to use pointers but must match the other forms
func (f *SystemMessage_v1) NewForm() Form {
	return NewSystemMessage_v1(LevelInfo, "", "")
}

func (f *SystemMessage_v1) FormVersion() string { return f.Version }
