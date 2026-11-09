import data from './ai-apps.json';

export type AppStep = {
  text: string;
  code?: string;
  bold?: string[];
  link?: { text: string; href: string };
  deeplink?: "cursor" | "vscode";
};

export type AppTable = {
  group: "chat" | "agents" | "computer";
  order: number;
  label?: string;
  where: string;
  deeplink?: "cursor" | "vscode";
};

// Matches the website's guide data. {url} and {key} in text are filled in later.
export type AppData = {
  slug: string;
  name: string;
  cloud: boolean;
  needs: string;
  signin: AppStep[];
  key: AppStep[];
  check: string;
  note: string;
  page: string | null;
  table?: AppTable;
  tested: string | null;
  agent?: boolean;
  clientIdHelper?: boolean;
};

export const APP_DATA = data as AppData[];
