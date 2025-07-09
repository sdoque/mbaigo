package forms

import (
	"reflect"
)

// Register the forms
func init() {
	FormTypeMap[messengerRegistrationVersion] = reflect.TypeOf(MessengerRegistration_v1{})
	FormTypeMap[systemMessageVersion] = reflect.TypeOf(SystemMessage_v1{})
}

////////////////////////////////////////////////////////////////////////////////

type MessengerRegistration_v1 struct {
	Host    string `json:"host"`
	Version string `json:"version"`
}

const messengerRegistrationVersion string = "MessengerRegistration_v1"

func NewMessengerRegistration_v1(host string) MessengerRegistration_v1 {
	return MessengerRegistration_v1{
		Host:    host,
		Version: messengerRegistrationVersion,
	}
}

func (f *MessengerRegistration_v1) NewForm() Form {
	new := NewMessengerRegistration_v1("")
	return &new
}

func (f *MessengerRegistration_v1) FormVersion() string { return f.Version }

////////////////////////////////////////////////////////////////////////////////

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

const systemMessageVersion string = "SystemMessage_v1"

func NewSystemMessage_v1(l MessageLevel, b string, s string) SystemMessage_v1 {
	return SystemMessage_v1{
		Level:   l,
		Body:    b,
		System:  s,
		Version: systemMessageVersion,
	}
}

// NewForm resets the form and defaults to using LevelInfo.
func (f *SystemMessage_v1) NewForm() Form {
	new := NewSystemMessage_v1(LevelInfo, "", "")
	return &new
}

func (f *SystemMessage_v1) FormVersion() string { return f.Version }
