package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// resetCmdHint is the one command that works in every Docker setup. The image
// has no shell, so the binary path /holyarrs has to be in it.
const resetCmdHint = "docker exec -it holyarrs /holyarrs reset-owner"

// ownerPrompt reads the new owner name and password. A terminal gets "Name:"
// and a hidden "Password:" asked twice, with another go on a mistake; piped
// input gets two lines, name first.
type ownerPrompt struct {
	in     io.Reader
	out    io.Writer
	tty    bool
	readPw func() (string, error) // hidden input on a terminal
	wait   time.Duration          // how long to wait for the name (docker exec -t without -i never sends any)
	code   io.Writer              // where --code prints the code (stdout)
}

var errNoTyping = errors.New("no typing")

// readLineWithin reads one line, or gives up after d. The reader goroutine is
// left behind on a timeout; the command exits right after.
func readLineWithin(rd *bufio.Reader, d time.Duration) (string, error) {
	type res struct {
		s   string
		err error
	}
	ch := make(chan res, 1)
	go func() { s, err := rd.ReadString('\n'); ch <- res{s, err} }()
	select {
	case r := <-ch:
		return r.s, r.err
	case <-time.After(d):
		return "", errNoTyping
	}
}

func resetOwner(args []string) int {
	fd := int(os.Stdin.Fd())
	p := ownerPrompt{in: os.Stdin, out: os.Stderr, code: os.Stdout, tty: term.IsTerminal(fd), wait: 3 * time.Minute, readPw: func() (string, error) {
		b, err := term.ReadPassword(fd)
		return string(b), err
	}}
	return p.run(args)
}

// run sets the owner's name and password, for when the password is lost.
// Every session and API key ends. It works while the server runs.
func (p ownerPrompt) run(args []string) int {
	codeOnly := false
	rest := args[:0:0]
	for _, a := range args {
		if a == "--code" || a == "-code" {
			codeOnly = true
			continue
		}
		rest = append(rest, a)
	}
	c, err := parseFlags(rest, p.out)
	if err != nil {
		return 2
	}
	s, err := database.Open(c.ConfigDir, database.Options{})
	if err != nil {
		fmt.Fprintf(p.out, "reset-owner: cannot open the database in %s: %v\n", c.ConfigDir, err)
		return 1
	}
	defer s.Close()
	ctx := context.Background()
	if codeOnly {
		return p.printCode(ctx, s)
	}
	minLen := accounts.MinPasswordLength(ctx, s.Read())

	fmt.Fprintln(p.out, "Reset the HolyArrs owner: choose the owner name and a new password.")
	var name, pw string
	if p.tty {
		if name, pw, err = p.ask(minLen); err != nil {
			fmt.Fprintf(p.out, "reset-owner: %v\n", err)
			return 1
		}
	} else {
		fmt.Fprintln(p.out, "Type the name, press Enter, then the new password and press Enter.")
		name, pw, err = accounts.ReadNewOwner(p.in)
		if errors.Is(err, accounts.ErrNoOwnerInput) {
			fmt.Fprintln(p.out, "reset-owner: nothing was typed in. Docker needs -it to pass your typing through:")
			fmt.Fprintln(p.out, "  "+resetCmdHint)
			fmt.Fprintln(p.out, "Use your container's name instead of holyarrs if it differs (docker ps lists them).")
			return 1
		}
		if err == nil {
			err = accounts.CheckNewOwner(name, pw, minLen)
		}
		if err != nil {
			fmt.Fprintf(p.out, "reset-owner: %v. Nothing was changed; run the command again.\n", err)
			return 1
		}
	}
	if err := accounts.ResetOwner(ctx, s, name, pw); err != nil {
		fmt.Fprintf(p.out, "reset-owner: %v. Nothing was changed.\n", err)
		return 1
	}
	fmt.Fprintf(p.out, "Done. Sign in as %q with the new password.\n", name)
	fmt.Fprintln(p.out, "Every device was signed out. Two-factor sign-in, API keys, open invites and Plex, Jellyfin, Emby and single sign-on links were removed.")
	fmt.Fprintln(p.out, "Afterwards, check allowed hosts and local networks in Settings.")
	return 0
}

// printCode makes a fresh one-time code for "Forgot password?" on the
// sign-in page and prints it alone on stdout, for scripts.
func (p ownerPrompt) printCode(ctx context.Context, s *database.Store) int {
	code, err := accounts.MakeOwnerResetCode(ctx, s, accounts.ResetCodeLife, time.Now())
	if err != nil {
		fmt.Fprintf(p.out, "reset-owner: %v\n", err)
		return 1
	}
	fmt.Fprintln(p.code, code)
	fmt.Fprintln(p.out, "Paste it on the sign-in page under Forgot password? It works once, for 15 minutes.")
	return 0
}

// ask reads the name and the password on a terminal, three tries at most.
func (p ownerPrompt) ask(minLen int) (string, string, error) {
	rd := bufio.NewReader(p.in)
	var last error
	for try := 0; try < 3; try++ {
		fmt.Fprint(p.out, "Name: ")
		line, err := readLineWithin(rd, p.wait)
		if errors.Is(err, errNoTyping) {
			fmt.Fprintln(p.out)
			return "", "", fmt.Errorf("nothing was typed in. If typing showed nothing, Docker needs -i as well: %s", resetCmdHint)
		}
		if err != nil && line == "" {
			return "", "", errors.New("no name came in")
		}
		name := strings.TrimSpace(line)
		fmt.Fprint(p.out, "New password (hidden): ")
		pw, err := p.readPw()
		fmt.Fprintln(p.out)
		if err != nil {
			return "", "", pwReadErr(err)
		}
		if last = accounts.CheckNewOwner(name, pw, minLen); last != nil {
			fmt.Fprintf(p.out, "%v. Try again.\n", last)
			continue
		}
		fmt.Fprint(p.out, "Same password again: ")
		again, err := p.readPw()
		fmt.Fprintln(p.out)
		if err != nil {
			return "", "", pwReadErr(err)
		}
		if again != pw {
			last = errors.New("the two passwords differ")
			fmt.Fprintln(p.out, "The two passwords differ. Try again.")
			continue
		}
		return name, pw, nil
	}
	return "", "", fmt.Errorf("%v. Nothing was changed", last)
}

// pwReadErr turns Ctrl+D (end of input) at a password prompt into plain words.
func pwReadErr(err error) error {
	if errors.Is(err, io.EOF) {
		return errors.New("no password came in. Nothing was changed")
	}
	return err
}

// printHelp lists the commands. Plain "holyarrs" starts the server.
func printHelp(w io.Writer) {
	fmt.Fprint(w, `HolyArrs commands (in Docker: docker exec -it <container> /holyarrs <command>):
  (none)          start the server
  reset-owner     set the owner's name and password when the password is lost
  reset-owner --code  print a one-time code for "Forgot password?" on the sign-in page
  doctor          check the config folder, database and folders
  restore <file>  restore a backup
  -healthcheck    exit 0 when the server answers
The image has no shell: docker exec ... sh fails with "OCI runtime exec failed".
Run these from the host's terminal, and keep /holyarrs in the command.
`)
}
