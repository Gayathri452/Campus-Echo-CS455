package email

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"
)

func TestConsoleEmailLogsDevelopmentOTPAndReturnsContextError(t *testing.T) {
	var output bytes.Buffer
	previousOutput := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previousOutput) })

	if err := (Console{}).SendOTP(context.Background(), "student@iitk.ac.in", "123456"); err != nil {
		t.Fatalf("SendOTP() error = %v", err)
	}
	if !strings.Contains(output.String(), "development OTP for student@iitk.ac.in: 123456") {
		t.Fatalf("development OTP log = %q", output.String())
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (Console{}).SendOTP(ctx, "student@iitk.ac.in", "123456"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled SendOTP() error = %v, want context.Canceled", err)
	}
}
