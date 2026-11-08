package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Errors from checking a Telegram Alert.
var (
	ErrTelegramToken = errors.New("paste the bot token from @BotFather in Telegram (it looks like 123456789:AAH…)")
	ErrTelegramChat  = errors.New("the chat ID is a number (negative for a group or channel) or a channel name like @mychannel")
)

// telegramMax is the most letters Telegram takes in one message.
const telegramMax = 4096

var (
	telegramTokenRe = regexp.MustCompile(`^[0-9]{3,20}:[A-Za-z0-9_-]{20,100}$`)
	telegramChatRe  = regexp.MustCompile(`^(-?[0-9]{1,20}|@[A-Za-z0-9_]{4,32})$`)
)

// CheckTelegramToken accepts a bot token as @BotFather gives it: the bot's
// number, a colon and its key. Nothing else may reach the request path.
func CheckTelegramToken(t string) error {
	if !telegramTokenRe.MatchString(strings.TrimSpace(t)) {
		return ErrTelegramToken
	}
	return nil
}

// CheckTelegramChat accepts a chat id (a number, negative for groups and
// channels) or a public channel name such as @mychannel.
func CheckTelegramChat(c string) error {
	if !telegramChatRe.MatchString(strings.TrimSpace(c)) {
		return ErrTelegramChat
	}
	return nil
}

// telegramBase is where the Telegram Bot API answers; HOLYARRS_TELEGRAM_URL
// points it at a stand-in for tests.
func telegramBase() string {
	if b := strings.TrimRight(strings.TrimSpace(os.Getenv("HOLYARRS_TELEGRAM_URL")), "/"); b != "" {
		return b
	}
	return "https://api.telegram.org"
}

// telegramAnswer is the part of a Bot API answer we read.
type telegramAnswer struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

// telegram posts m as plain text to the Alert's chat. The request path holds
// the bot token, so neither the address nor the token ever goes into an
// error.
func (s *Sender) telegram(ctx context.Context, a domain.Alert, m Message) error {
	token, err := s.secret(ctx, a.PasswordSecretID)
	if err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	if CheckTelegramToken(token) != nil {
		return fail("the Telegram bot token is missing; type it again", nil)
	}
	if CheckTelegramChat(a.ChatID) != nil {
		return ErrTelegramChat
	}
	text := cutRunes(strings.TrimSpace(m.Subject+"\n"+m.Text), telegramMax)
	body, _ := json.Marshal(map[string]any{"chat_id": strings.TrimSpace(a.ChatID), "text": text, "disable_web_page_preview": true})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, telegramBase()+"/bot"+token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return fail("the Telegram address does not look right", nil)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client(a).Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		if errors.Is(err, outbound.ErrAddressRefused) {
			return fail("the Telegram address is on the local network, which this Alert may not reach", err)
		}
		return again("Telegram did not answer", err)
	}
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if closeErr := resp.Body.Close(); readErr == nil {
		readErr = closeErr
	}
	if readErr != nil {
		return again("Telegram's answer could not be read", readErr)
	}
	if resp.StatusCode/100 == 2 {
		return nil
	}
	var ans telegramAnswer
	_ = json.Unmarshal(raw, &ans)
	// Telegram's own reason is only logged (detail); it never holds the token.
	var detail error
	if ans.Description != "" {
		detail = errors.New(ans.Description)
	}
	switch code := resp.StatusCode; {
	case code == http.StatusTooManyRequests:
		f := &failure{public: "Telegram asked HolyArrs to slow down", detail: detail, retry: true}
		if n := ans.Parameters.RetryAfter; n > 0 {
			f.public = fmt.Sprintf("Telegram asked HolyArrs to wait %d s", n)
			f.wait = time.Duration(n) * time.Second
		}
		return f
	case code >= 500:
		return again(fmt.Sprintf("Telegram answered %d", code), detail)
	case code == http.StatusUnauthorized, code == http.StatusNotFound:
		return fail("Telegram refused the bot token", detail)
	case code == http.StatusForbidden:
		return fail("Telegram would not let the bot write to that chat (start a chat with the bot, or add it to the group or channel)", detail)
	case code == http.StatusBadRequest:
		if strings.Contains(strings.ToLower(ans.Description), "chat not found") {
			return fail("Telegram could not find that chat", detail)
		}
		return fail("Telegram refused the message", detail)
	}
	return statusFailure("Telegram", resp.StatusCode)
}
