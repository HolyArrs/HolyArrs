// ntfy, Gotify and Pushover share the app_token field, and ntfy and Gotify share server_url. When the Kind changes while adding, those are emptied so a token typed for one app is never saved, and sent, as another app's.
export function switchKind<F extends { kind: string; app_token: string; server_url: string; clear_app_token: boolean }>(form: F, kind: F['kind']): F {
	if (kind === form.kind) return form;
	return { ...form, kind, app_token: '', server_url: '', clear_app_token: false };
}
