import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';

// My account is one page inside Settings; /account keeps working and keeps its query, such as ?sso=finish after linking single sign-on.
export function load({ url }: { url: URL }) {
	redirect(307, resolve('/settings/account') + url.search + url.hash);
}
