package email

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
)

func TestSMTPDeliveryUsesConfiguredEnvelopeAndMessage(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	type captured struct {
		from    string
		to      string
		message string
		err     error
	}
	result := make(chan captured, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			result <- captured{err: err}
			return
		}
		defer connection.Close()
		reader := bufio.NewReader(connection)
		write := func(response string) error {
			_, err := fmt.Fprint(connection, response+"\r\n")
			return err
		}
		if err := write("220 test SMTP"); err != nil {
			result <- captured{err: err}
			return
		}
		var got captured
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				got.err = err
				result <- got
				return
			}
			command := strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(command, "EHLO "):
				err = write("250-test SMTP")
				if err == nil {
					err = write("250 SIZE 100000")
				}
			case strings.HasPrefix(command, "MAIL FROM:"):
				got.from = command
				err = write("250 sender accepted")
			case strings.HasPrefix(command, "RCPT TO:"):
				got.to = command
				err = write("250 recipient accepted")
			case command == "DATA":
				err = write("354 send message")
				for err == nil {
					var messageLine string
					messageLine, err = reader.ReadString('\n')
					if err != nil {
						break
					}
					if strings.TrimSpace(messageLine) == "." {
						break
					}
					got.message += messageLine
				}
				if err == nil {
					err = write("250 message accepted")
				}
			case command == "QUIT":
				_ = write("221 closing connection")
				result <- got
				return
			default:
				err = fmt.Errorf("unexpected SMTP command %q", command)
			}
			if err != nil {
				got.err = err
				result <- got
				return
			}
		}
	}()

	err = (SMTP{Host: host, Port: port, From: "noreply@example.test"}).
		SendOTP(context.Background(), "student@example.test", "123456")
	if err != nil {
		t.Fatalf("SendOTP() error = %v", err)
	}
	got := <-result
	if got.err != nil {
		t.Fatalf("SMTP test server: %v", got.err)
	}
	if got.from != "MAIL FROM:<noreply@example.test>" || got.to != "RCPT TO:<student@example.test>" {
		t.Fatalf("SMTP envelope = (%q, %q)", got.from, got.to)
	}
	for _, line := range []string{
		"To: student@example.test\r\n",
		"From: noreply@example.test\r\n",
		"Subject: CampusEcho verification code\r\n",
		"Your CampusEcho verification code is: 123456\r\n",
	} {
		if !strings.Contains(got.message, line) {
			t.Errorf("SMTP message missing %q: %q", line, got.message)
		}
	}
}

func TestSMTPDeliveryRejectsInvalidSetupAndUnavailableServer(t *testing.T) {
	sender := SMTP{Host: "127.0.0.1", Port: "25", From: "noreply@example.test"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sender.SendOTP(ctx, "student@example.test", "123456"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled SendOTP() error = %v, want context.Canceled", err)
	}
	for _, test := range []SMTP{
		{Host: "localhost", Port: "25"},
		{Host: "localhost", From: "noreply@example.test"},
		{Port: "25", From: "noreply@example.test"},
	} {
		if err := test.SendOTP(context.Background(), "student@example.test", "123456"); err == nil || !strings.Contains(err.Error(), "configuration is incomplete") {
			t.Errorf("incomplete config error = %v", err)
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().(*net.TCPAddr)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	sender.Port = fmt.Sprint(address.Port)
	if err := sender.SendOTP(context.Background(), "student@example.test", "123456"); err == nil || !strings.Contains(err.Error(), "connect to SMTP server") {
		t.Fatalf("unavailable SMTP server error = %v", err)
	}
}

func TestSMTPDeliveryReportsServerRejections(t *testing.T) {
	tests := []struct {
		name        string
		stage       string
		wantMessage string
		credentials bool
	}{
		{name: "invalid greeting", stage: "greeting", wantMessage: "create SMTP client"},
		{name: "authentication rejected", stage: "auth", wantMessage: "authenticate with SMTP server", credentials: true},
		{name: "sender rejected", stage: "mail", wantMessage: "set SMTP sender"},
		{name: "recipient rejected", stage: "recipient", wantMessage: "set SMTP recipient"},
		{name: "message data rejected", stage: "data", wantMessage: "start SMTP message"},
		{name: "message delivery rejected", stage: "message", wantMessage: "send SMTP message"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			host, port, err := net.SplitHostPort(listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}

			serverDone := make(chan error, 1)
			go func() {
				connection, err := listener.Accept()
				if err != nil {
					serverDone <- err
					return
				}
				defer connection.Close()
				reader := bufio.NewReader(connection)
				write := func(response string) error {
					_, err := fmt.Fprint(connection, response+"\r\n")
					return err
				}
				if test.stage == "greeting" {
					serverDone <- write("500 invalid greeting")
					return
				}
				if err := write("220 test SMTP"); err != nil {
					serverDone <- err
					return
				}
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						serverDone <- err
						return
					}
					command := strings.TrimSpace(line)
					switch {
					case strings.HasPrefix(command, "EHLO "):
						err = write("250-test SMTP")
						if err == nil && test.stage == "auth" {
							err = write("250-AUTH PLAIN")
						}
						if err == nil {
							err = write("250 SIZE 100000")
						}
					case strings.HasPrefix(command, "AUTH "):
						err = write("535 authentication rejected")
						serverDone <- err
						return
					case strings.HasPrefix(command, "MAIL FROM:"):
						if test.stage == "mail" {
							err = write("550 sender rejected")
							serverDone <- err
							return
						}
						err = write("250 sender accepted")
					case strings.HasPrefix(command, "RCPT TO:"):
						if test.stage == "recipient" {
							err = write("550 recipient rejected")
							serverDone <- err
							return
						}
						err = write("250 recipient accepted")
					case command == "DATA":
						if test.stage == "data" {
							err = write("550 data rejected")
							serverDone <- err
							return
						}
						err = write("354 send message")
						for err == nil {
							var messageLine string
							messageLine, err = reader.ReadString('\n')
							if err != nil || strings.TrimSpace(messageLine) == "." {
								break
							}
						}
						if err == nil {
							err = write("550 message rejected")
							serverDone <- err
							return
						}
					default:
						serverDone <- fmt.Errorf("unexpected SMTP command %q", command)
						return
					}
					if err != nil {
						serverDone <- err
						return
					}
				}
			}()

			sender := SMTP{Host: host, Port: port, From: "noreply@example.test"}
			if test.credentials {
				sender.Username = "smtp-user"
				sender.Password = "smtp-password"
			}
			err = sender.SendOTP(context.Background(), "student@example.test", "123456")
			if err == nil || !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf("SendOTP() error = %v, want message containing %q", err, test.wantMessage)
			}
			if serverErr := <-serverDone; serverErr != nil {
				t.Fatalf("SMTP test server: %v", serverErr)
			}
		})
	}
}
