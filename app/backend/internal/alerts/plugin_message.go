package alerts

import "context"

// SendPlugin sends a plugin's message through every Alert that has plugin
// messages ticked, its subject starting with the plugin's name, so nobody
// takes it for a message from HolyArrs itself.
func (s *Sender) SendPlugin(ctx context.Context, plugin, subject, text string) error {
	lang := s.lang(ctx)
	return s.Send(ctx, Message{lang: lang, Moment: MomentPlugin, Subject: say(lang, "plugin_from", "plugin", plugin) + ": " + subject, Text: text})
}
