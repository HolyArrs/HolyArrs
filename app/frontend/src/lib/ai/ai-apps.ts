// Setup for every AI app, the same words as the website guides. The words live in
// ai-apps.json and must match the website's guide data. {url} and {key} are filled in.
import data from './ai-apps.json';

export type AppStep = {
  text: string;
  code?: string;
  /** UI labels in the text that show in bold. */
  bold?: string[];
  /** A link shown under the step. */
  link?: { text: string; href: string };
  /** The app's own one-click install button (sign-in only). */
  deeplink?: "cursor" | "vscode";
};

export type AppTable = {
  group: "chat" | "agents" | "computer";
  order: number;
  /** The row name when it differs from the app name. */
  label?: string;
  /** Where the setting is; `code` in backticks, <address> is the AI address. */
  where: string;
  deeplink?: "cursor" | "vscode";
};

export type AppData = {
  slug: string;
  name: string;
  /** Connects from the maker's servers: needs a public https address. */
  cloud: boolean;
  needs: string;
  /** Steps on https or this computer (browser sign-in). */
  signin: AppStep[];
  /** Steps on a plain http home address (AI key). */
  key: AppStep[];
  check: string;
  note: string;
  /** Its guide page under /docs/ai/, if it has one. */
  page: string | null;
  /** Its row in the Where to add HolyArrs table, if it has one. */
  table?: AppTable;
  /** The day it was tested with HolyArrs (YYYY-MM-DD), or null. */
  tested: string | null;
  /** An agent that can set itself up from the agent guide. */
  agent?: boolean;
  /** The app does not register itself: Settings makes it a client ID. */
  clientIdHelper?: boolean;
};

export const APP_DATA = data as AppData[];
