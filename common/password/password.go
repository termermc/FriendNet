package password

import (
	"errors"
	"fmt"
	"strings"

	"friendnet.org/common"
	mcfpassword "github.com/termermc/go-mcf-password"
)

const specialChars = "!?@#$%^&*()_+-=[]{}|;':\",.<>/"

// Error is returned by Requirements.Check when the password is invalid.
type Error struct {
	// Inner is the individual password validation errors.
	Inner []error
}

func (e Error) Error() string {
	const prefix = "password validation failed: "

	totalLen := len(prefix)
	for _, err := range e.Inner {
		totalLen += len(err.Error()) + 1
	}
	sb := strings.Builder{}
	sb.Grow(totalLen)
	sb.WriteString(prefix)
	for i, err := range e.Inner {
		sb.WriteString(err.Error())
		if i < len(e.Inner)-1 {
			sb.WriteString("; ")
		}
	}

	return sb.String()
}

func (e Error) Unwrap() []error {
	return e.Inner
}

// ErrEmptyPassword is returned when a password is empty.
var ErrEmptyPassword = errors.New("empty password")

// LengthError is returned by WithMinLen when the password is too long or too short.
type LengthError struct {
	Expected int
	Actual   int
}

func (e LengthError) Error() string {
	if e.Actual > e.Expected {
		return fmt.Sprintf("password was %d characters long but must be at most %d", e.Actual, e.Expected)
	}

	return fmt.Sprintf("password was %d characters long but must be at least %d", e.Actual, e.Expected)
}

// ErrContainsUsername is returned by WithCannotContainUsername when the password contains the username.
var ErrContainsUsername = errors.New("password cannot contain username")

// ErrNoNumber is returned by WithRequireNumber when the password does not contain a number.
var ErrNoNumber = errors.New("password must contain a number")

// ErrNoUppercase is returned by WithRequireUppercase when the password does not contain an uppercase letter.
var ErrNoUppercase = errors.New("password must contain an uppercase letter")

// ErrNoSpecialChar is returned by WithRequireSpecialChar when the password does not contain a special character.
var ErrNoSpecialChar = errors.New("password must contain a special character (one of " + specialChars + ")")

// Checker is a function that checks whether a password is valid.
// It returns an error if the password is invalid, or nil if valid.
type Checker func(username common.NormalizedUsername, password string) error

// Requirements is a collection of password requirements.
// It can verify that passwords adhere to the requirements.
// It by default does not allow empty passwords.
// The empty value enforces no requirements other than no empty passwords.
type Requirements struct {
	checkers []Checker
}

// Check checks whether the specified password is valid.
// It returns all errors that occurred during the check, wrapped in Error,
// or nil if no validation errors occurred.
// You can use errors.Is or errors.As to check for specific errors.
func (r *Requirements) Check(username common.NormalizedUsername, password string) error {
	errs := make([]error, 0)

	if len(password) == 0 {
		errs = append(errs, ErrEmptyPassword)
	}

	for _, checker := range r.checkers {
		if err := checker(username, password); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return Error{Inner: errs}
	}

	return nil
}

// NewRequirements returns a new Requirements with the specified checkers.
// The only requirement enforced by default is that passwords cannot be empty.
//
// Example:
//
//	requirements := password.NewRequirements(
//		password.WithMinLen(8),
//		password.WithMaxLen(32),
//		password.WithCannotContainUsername(),
//		password.WithRequireNumber(),
//		password.WithRequireUppercase(),
//		password.WithRequireSpecialChar(),
//	)
func NewRequirements(checkers ...Checker) Requirements {
	return Requirements{
		checkers: checkers,
	}
}

// WithMinLen returns a Checker that requires the password to be at least min characters long.
// Returns a LengthError if the password is too short.
func WithMinLen(min int) Checker {
	return func(username common.NormalizedUsername, password string) error {
		if len(password) < min {
			return LengthError{
				Expected: min,
				Actual:   len(password),
			}
		}
		return nil
	}
}

// WithMaxLen returns a Checker that requires the password to be at most max characters long.
// Returns a LengthError if the password is too long.
func WithMaxLen(max int) Checker {
	return func(username common.NormalizedUsername, password string) error {
		if len(password) > max {
			return LengthError{
				Expected: max,
				Actual:   len(password),
			}
		}
		return nil
	}
}

// WithCannotContainUsername returns a Checker that requires the password to not contain the username.
// Returns ErrContainsUsername if the password contains the username.
func WithCannotContainUsername() Checker {
	return func(username common.NormalizedUsername, password string) error {
		if strings.Contains(strings.ToLower(password), username.String()) {
			return ErrContainsUsername
		}
		return nil
	}
}

// WithRequireNumber returns a Checker that requires the password to contain a number.
// Returns ErrNoNumber if the password does not contain a number.
func WithRequireNumber() Checker {
	return func(username common.NormalizedUsername, password string) error {
		if !strings.ContainsAny(password, "0123456789") {
			return ErrNoNumber
		}
		return nil
	}
}

// WithRequireUppercase returns a Checker that requires the password to contain an uppercase letter.
// Returns ErrNoUppercase if the password does not contain an uppercase letter.
func WithRequireUppercase() Checker {
	return func(username common.NormalizedUsername, password string) error {
		if !strings.ContainsAny(password, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
			return ErrNoUppercase
		}
		return nil
	}
}

// WithRequireSpecialChar returns a Checker that requires the password to contain a special character.
// Returns ErrNoSpecialChar if the password does not contain a special character.
func WithRequireSpecialChar() Checker {
	return func(username common.NormalizedUsername, password string) error {
		if !strings.ContainsAny(password, specialChars) {
			return ErrNoSpecialChar
		}
		return nil
	}
}

// HashWithRequirements hashes the specified password with the specified requirements.
// Returns an error if the password does not adhere to the requirements or if hashing fails.
func HashWithRequirements(username common.NormalizedUsername, password string, requirements Requirements) (string, error) {
	if err := requirements.Check(username, password); err != nil {
		return "", err
	}

	hash, err := mcfpassword.HashPassword(password)
	if err != nil {
		return "", err
	}

	return hash, nil
}
