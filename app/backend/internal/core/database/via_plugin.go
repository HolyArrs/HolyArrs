package database

import "context"

type viaPluginKey struct{}

// WithViaPlugin marks ctx as a call a plugin made (for a person or on its
// own); records written under it keep the plugin's id in via_plugin.
func WithViaPlugin(ctx context.Context, plugin string) context.Context {
	if plugin == "" {
		return ctx
	}
	return context.WithValue(ctx, viaPluginKey{}, plugin)
}

// ViaPlugin is the plugin ctx was marked with, or nil.
func ViaPlugin(ctx context.Context) *string {
	if ctx == nil {
		return nil
	}
	if v, ok := ctx.Value(viaPluginKey{}).(string); ok && v != "" {
		return &v
	}
	return nil
}
