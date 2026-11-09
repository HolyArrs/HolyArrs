package secretsfile

import (
	"context"
	"testing"
)

func TestSMTPDryRunRejectsInvalidSettings(t *testing.T) {
	for _, tc := range []struct {
		name, host, port, from, want string
	}{
		{"host", "https://your-mail.invalid", "587", "your-sender@example.invalid", "host must be a server name without a scheme or port"},
		{"port", "your-mail.invalid", "65536", "your-sender@example.invalid", "port must be between 1 and 65535"},
		{"from", "your-mail.invalid", "587", "your-display-name", "from must be one plain email address"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, dry := range []bool{true, false} {
				d := deps(t)
				before := count(t, d, "SELECT count(*) FROM secrets")
				f := parse(t, "version: 1\n")
				f.SMTP.Host, f.SMTP.Port, f.SMTP.From = tc.host, tc.port, tc.from
				f.SMTP.Password = "your-mail-password"
				ls := Apply(context.Background(), d, f, dry)
				if AllOK(ls) || results(ls)["smtp"] != "failed: "+tc.want {
					t.Fatalf("dry=%t: %v", dry, ls)
				}
				if count(t, d, "SELECT count(*) FROM alerts") != 0 || count(t, d, "SELECT count(*) FROM secrets") != before {
					t.Fatal("invalid mail settings were stored")
				}
				noValues(t, d, f, ls)
			}
		})
	}
}
