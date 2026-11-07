// The add dialogs' one copy of the saved Movies and TV defaults for this
// visit: pages forget it when they open, the dialogs wait for it.
import { api } from './core/api.ts';
import { defaultsLoader } from './title-defaults.ts';
import type { TitleDefaults } from './mediafiles/index.ts';

export const savedDefaults = defaultsLoader((p) => api.get<TitleDefaults>(p));
