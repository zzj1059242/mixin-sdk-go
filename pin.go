package mixin

import (
	"context"
	"fmt"
	"regexp"
)

func (c *Client) VerifyPin(ctx context.Context, pin string) error {
	body := map[string]interface{}{
		"pin": c.EncryptPin(pin),
	}

	return c.Post(ctx, "/pin/verify", body, nil)
}

func (c *Client) ModifyPin(ctx context.Context, pin, newPin string) error {
	body := map[string]interface{}{}

	if pin != "" {
		body["old_pin"] = c.EncryptPin(pin)
	}

	body["pin"] = c.EncryptPin(newPin)

	return c.Post(ctx, "/pin/update", body, nil)
}

var (
	pinRegex = regexp.MustCompile(`^\d{6}$`)
)

// ValidatePinPattern validate the pin with pinRegex
func ValidatePinPattern(pin string) error {
	if !pinRegex.MatchString(pin) {
		return fmt.Errorf("pin must match regex pattern %q", pinRegex.String())
	}

	return nil
}
