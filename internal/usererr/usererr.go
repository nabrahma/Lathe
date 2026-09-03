// Package usererr turns engine failures into sentences a person can act on.
//
// "exit status 137" is not a message. Every error that reaches the interface
// passes through here and comes out as plain language plus, wherever one
// exists, a next action. An unmapped error still gets an honest message and a
// way to copy the technical detail for a bug report.
package usererr

import (
	"errors"
	"fmt"
	"strings"
)

// Code identifies a failure class. It is what the UI switches on to decide
// which follow-up control to render: a password field, a retry button.
type Code string

// The failure classes Lathe recognises.
const (
	CodeUnknown          Code = "unknown"
	CodePasswordRequired Code = "password_required"
	CodePasswordWrong    Code = "password_wrong"
	CodeCorruptInput     Code = "corrupt_input"
	CodeUnsupportedInput Code = "unsupported_input"
	CodeEmptyInput       Code = "empty_input"
	CodeNoTextFound      Code = "no_text_found"
	CodeOutOfMemory      Code = "out_of_memory"
	CodeDiskFull         Code = "disk_full"
	CodeFileLocked       Code = "file_locked"
	CodeNotWritable      Code = "not_writable"
	CodeComponentMissing Code = "component_missing"
	CodeCancelled        Code = "cancelled"
	CodeTimeout          Code = "timeout"
	CodeInvalidOptions   Code = "invalid_options"
	CodeOutputInvalid    Code = "output_invalid"
)

// Action is a follow-up the interface can offer. A dead end with no next step
// is a failure of the interface, not of the user.
type Action string

// The follow-up actions an error can suggest.
const (
	ActionRetry         Action = "retry"
	ActionEnterPassword Action = "enter_password"
	ActionChooseFile    Action = "choose_file"
	ActionChangeOption  Action = "change_option"
	ActionFreeSpace     Action = "free_space"
	ActionDownload      Action = "download"
	ActionCopyDetails   Action = "copy_details"
)

// Error is a failure as the user sees it.
type Error struct {
	Code Code `json:"code"`
	// Message is one or two sentences, sentence case, no jargon and no
	// library names. It is never uppercased by the interface.
	Message string `json:"message"`
	// Detail is the raw engine output, shown only behind "Copy details".
	Detail string `json:"detail,omitempty"`
	// Actions are the follow-ups to offer, most useful first.
	Actions []Action `json:"actions,omitempty"`
	// File is the input this failure relates to, when it is one of several.
	File string `json:"file,omitempty"`

	cause error
}

func (e *Error) Error() string { return e.Message }

// Unwrap exposes the underlying cause for errors.Is and errors.As.
func (e *Error) Unwrap() error { return e.cause }

// New builds a user-facing error directly, for failures Lathe raises itself.
func New(code Code, message string, actions ...Action) *Error {
	return &Error{Code: code, Message: message, Actions: actions}
}

// Wrap attaches a cause, whose text becomes the copyable detail.
func Wrap(cause error, code Code, message string, actions ...Action) *Error {
	e := New(code, message, actions...)
	e.cause = cause
	if cause != nil {
		e.Detail = cause.Error()
	}
	return e
}

// WithFile records which input a failure belongs to.
func (e *Error) WithFile(name string) *Error {
	e.File = name
	return e
}

// rule maps a substring of engine output to a user-facing failure. Order
// matters: the first match wins, so specific patterns precede general ones.
type rule struct {
	match   string
	code    Code
	message string
	actions []Action
}

// engineRules is the translation table. Each entry exists because the raw text
// on the left was, at some point, shown to a person and meant nothing to them.
var engineRules = []rule{
	// pdfcpu
	{"please provide the correct password", CodePasswordWrong,
		"That password did not open the PDF. Check it and try again.",
		[]Action{ActionEnterPassword}},
	{"missing password", CodePasswordRequired,
		"This PDF is password-protected. Enter the password to continue.",
		[]Action{ActionEnterPassword}},
	{"encrypted", CodePasswordRequired,
		"This PDF is password-protected. Enter the password to continue.",
		[]Action{ActionEnterPassword}},
	{"not a pdf file", CodeUnsupportedInput,
		"This file is not a PDF, despite its name. Pick a different file.",
		[]Action{ActionChooseFile}},
	{"xref table", CodeCorruptInput,
		"This PDF appears to be damaged and can't be read.",
		[]Action{ActionChooseFile, ActionCopyDetails}},
	{"corrupt", CodeCorruptInput,
		"This file appears to be damaged and can't be read.",
		[]Action{ActionChooseFile, ActionCopyDetails}},

	// FFmpeg
	{"invalid data found when processing input", CodeCorruptInput,
		"This file appears to be damaged and can't be read.",
		[]Action{ActionChooseFile, ActionCopyDetails}},
	{"no such file or directory", CodeUnsupportedInput,
		"That file could not be found. It may have been moved or renamed.",
		[]Action{ActionChooseFile}},
	{"decoder not found", CodeUnsupportedInput,
		"This file uses a format Lathe can't read yet.",
		[]Action{ActionChooseFile, ActionCopyDetails}},
	{"does not contain any stream", CodeCorruptInput,
		"There is no video or audio in this file.",
		[]Action{ActionChooseFile}},

	// Tesseract
	{"empty page", CodeNoTextFound,
		"No text was found in this image. If the text is small or blurry, try a clearer photo.",
		[]Action{ActionChooseFile, ActionChangeOption}},
	{"failed loading language", CodeComponentMissing,
		"That language pack isn't installed yet. Download it to read text in this language.",
		[]Action{ActionDownload}},

	// LibreOffice
	{"source file could not be loaded", CodeCorruptInput,
		"This document couldn't be opened. It may be damaged, or made by software LibreOffice doesn't recognise.",
		[]Action{ActionChooseFile, ActionCopyDetails}},

	// Operating system
	{"no space left on device", CodeDiskFull,
		"There isn't enough space to save the result. Free up some space and try again.",
		[]Action{ActionFreeSpace, ActionRetry}},
	{"not enough space", CodeDiskFull,
		"There isn't enough space to save the result. Free up some space and try again.",
		[]Action{ActionFreeSpace, ActionRetry}},
	{"being used by another process", CodeFileLocked,
		"That file is open in another program. Close it and try again.",
		[]Action{ActionRetry}},
	{"sharing violation", CodeFileLocked,
		"That file is open in another program. Close it and try again.",
		[]Action{ActionRetry}},
	{"permission denied", CodeNotWritable,
		"Lathe isn't allowed to write to that folder. Choose a different one.",
		[]Action{ActionChangeOption}},
	{"access is denied", CodeNotWritable,
		"Lathe isn't allowed to write to that folder. Choose a different one.",
		[]Action{ActionChangeOption}},
	{"read-only file system", CodeNotWritable,
		"That folder can't be written to. Choose a different one.",
		[]Action{ActionChangeOption}},
	{"cannot allocate memory", CodeOutOfMemory,
		"The conversion ran out of memory. Try a smaller file, or close other applications.",
		[]Action{ActionRetry, ActionChooseFile}},
	{"out of memory", CodeOutOfMemory,
		"The conversion ran out of memory. Try a smaller file, or close other applications.",
		[]Action{ActionRetry, ActionChooseFile}},
}

// exitCodes covers the numeric failures that have a known meaning. 137 is the
// one users actually hit: the kernel killed the process for using too much
// memory, and the number tells them nothing.
var exitCodes = map[int]rule{
	137: {code: CodeOutOfMemory,
		message: "The conversion ran out of memory. Try a smaller file, or close other applications.",
		actions: []Action{ActionRetry, ActionChooseFile}},
	139: {code: CodeCorruptInput,
		message: "The conversion stopped unexpectedly, usually because the file is damaged.",
		actions: []Action{ActionChooseFile, ActionCopyDetails}},
}

// Translate converts any error into something worth showing. It never returns
// nil for a non-nil input, and never lets raw engine text reach the main flow.
func Translate(err error) *Error {
	if err == nil {
		return nil
	}

	// Already translated: keep the specific message.
	var ue *Error
	if errors.As(err, &ue) {
		return ue
	}

	text := strings.ToLower(err.Error())
	for _, r := range engineRules {
		if strings.Contains(text, r.match) {
			return &Error{Code: r.code, Message: r.message, Actions: r.actions,
				Detail: err.Error(), cause: err}
		}
	}

	if code, ok := exitCodeOf(text); ok {
		if r, known := exitCodes[code]; known {
			return &Error{Code: r.code, Message: r.message, Actions: r.actions,
				Detail: err.Error(), cause: err}
		}
	}

	return &Error{
		Code:    CodeUnknown,
		Message: "Something went wrong during the conversion, and the original file is untouched.",
		Detail:  err.Error(),
		Actions: []Action{ActionRetry, ActionCopyDetails},
		cause:   err,
	}
}

// exitCodeOf pulls the number out of Go's "exit status N" phrasing.
func exitCodeOf(text string) (int, bool) {
	const marker = "status "
	i := strings.Index(text, marker)
	if i < 0 {
		return 0, false
	}
	var code int
	if _, err := fmt.Sscanf(text[i+len(marker):], "%d", &code); err != nil {
		return 0, false
	}
	return code, true
}
