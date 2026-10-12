package email

import (
	"context"
	"log"
)

// Console is for local development only; it never sends real email.
type Console struct{}

func (Console) SendOTP(ctx context.Context, recipient, code string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	log.Printf("development OTP for %s: %s", recipient, code)
	return nil
}
