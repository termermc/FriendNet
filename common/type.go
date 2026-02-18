package common

import (
	"errors"
	"strings"
)

// ErrInvalidUsername is returned when an invalid username is encountered.
var ErrInvalidUsername = errors.New("invalid username")

// ErrInvalidRoomName is returned when an invalid room name is encountered.
var ErrInvalidRoomName = errors.New("invalid room name")

// NormalizedUsername is a normalized, valid username.
//
// A valid, normalized username adheres to the following rules:
//   - Only contains ASCII letters, numbers and underscores.
//   - Always lowercase.
//   - 1-16 characters long (inclusive).
type NormalizedUsername struct {
	string
}

// IsZero returns whether the NormalizedUsername is a zero value.
// If so, it MUST NOT be used.
// Calling NormalizedUsername.String on a zero value will panic.
func (u NormalizedUsername) IsZero() bool {
	return u.string == ""
}

// String returns the string representation of the username.
// If calling on a zero value, panics.
// If unsure, call NormalizedUsername.IsZero first.
func (u NormalizedUsername) String() string {
	if u.IsZero() {
		panic("tried to call String() on a zero NormalizedUsername")
	}

	return u.string
}

// ZeroNormalizedUsername is the zero value of NormalizedUsername.
// It is invalid.
var ZeroNormalizedUsername = NormalizedUsername{}

// UncheckedCreateNormalizedUsername creates a NormalizedUsername without checking input.
// DO NOT use this on untrusted input.
func UncheckedCreateNormalizedUsername(str string) NormalizedUsername {
	return NormalizedUsername{strings.ToLower(str)}
}

// NormalizeUsername normalizes and validates the specified username string.
// If the string is a valid username, returns the normalized version and true.
// If the string is not a valid username, returns ZeroNormalizedUsername (should not be used) and false.
func NormalizeUsername(str string) (NormalizedUsername, bool) {
	if len(str) < 1 || len(str) > 16 {
		return ZeroNormalizedUsername, false
	}

	lower := strings.ToLower(str)
	for _, c := range lower {
		if (c >= '0' && c <= '9') ||
			(c >= 'a' && c <= 'z') ||
			c == '_' {
			continue
		}

		return ZeroNormalizedUsername, false
	}

	return NormalizedUsername{lower}, true
}

// NormalizedRoomName is a normalized, valid room name.
//
// A valid, normalized room name adheres to the following rules:
//   - Only contains ASCII letters, numbers and underscores.
//   - Always lowercase.
//   - 1-16 characters long (inclusive).
type NormalizedRoomName struct {
	string
}

// IsZero returns whether the NormalizedRoomName is a zero value.
// If so, it MUST NOT be used.
// Calling NormalizedRoomName.String on a zero value will panic.
func (u NormalizedRoomName) IsZero() bool {
	return u.string == ""
}

// String returns the string representation of the room name.
// If calling on a zero value, panics.
// If unsure, call NormalizedRoomName.IsZero first.
func (u NormalizedRoomName) String() string {
	if u.IsZero() {
		panic("tried to call String() on a zero NormalizedRoomName")
	}

	return u.string
}

// ZeroNormalizedRoomName is the zero value of NormalizedRoomName.
// It is invalid.
var ZeroNormalizedRoomName = NormalizedRoomName{}

// UncheckedCreateNormalizedRoomName creates a NormalizedRoomName without checking input.
// DO NOT use this on untrusted input.
func UncheckedCreateNormalizedRoomName(str string) NormalizedRoomName {
	return NormalizedRoomName{strings.ToLower(str)}
}

// NormalizeRoomName normalizes and validates the specified room name string.
// If the string is a valid room name, returns the normalized version and true.
// If the string is not a valid room name, returns ZeroNormalizedRoomName (should not be used) and false.
func NormalizeRoomName(str string) (NormalizedRoomName, bool) {
	if len(str) < 1 || len(str) > 16 {
		return ZeroNormalizedRoomName, false
	}

	lower := strings.ToLower(str)
	for _, c := range lower {
		if (c >= '0' && c <= '9') ||
			(c >= 'a' && c <= 'z') ||
			c == '_' {
			continue
		}

		return ZeroNormalizedRoomName, false
	}

	return UncheckedCreateNormalizedRoomName(lower), true
}
