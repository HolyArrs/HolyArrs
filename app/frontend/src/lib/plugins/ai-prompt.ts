// The short prompt behind "Build with AI" on the Add sheet. It stays in English (AI assistants read it, not people).
// It only points at the hosted guide, so the rules can change without an app update.
export const PLUGINS_REPO = 'https://github.com/HolyArrs/HolyArrs-Plugins';

/** The full guide the prompt points at; mirrored from the plugins repo's docs/for-ai-assistants.md, which is the only place it is written. */
export const AI_GUIDE_URL = 'https://holyarrs.com/ai/plugin.md';

export const AI_PROMPT = `Read ${AI_GUIDE_URL} and help me make a HolyArrs plugin. My idea: <describe it here>`;

/** The developer links on the Add sheet. */
export const DEV_STEPS = {
	guide: `${PLUGINS_REPO}/blob/main/docs/making-a-plugin.md`,
	template: `${PLUGINS_REPO}/tree/main/docs/template`
};
