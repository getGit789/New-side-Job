package auth

import (
	_ "embed"
	"errors"
	"strings"
	"sync"
)

// common-passwords.txt is the SecLists "10k most common" list (MIT), shipped with the product (contract §8).
//
//go:embed common-passwords.txt
var commonList string

var (
	commonOnce sync.Once
	common     map[string]bool
)

var ErrWeakPassword = errors.New("password must be 12–200 characters and not a commonly used password")

// CheckPassword applies the contract §8 rule: 12–200 characters and not on the shipped breached list.
func CheckPassword(pw string) error {
	commonOnce.Do(func() {
		common = map[string]bool{}
		for _, line := range strings.Split(commonList, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				common[line] = true
			}
		}
	})
	if len(pw) < 12 || len(pw) > 200 || common[strings.ToLower(pw)] {
		return ErrWeakPassword
	}
	return nil
}
