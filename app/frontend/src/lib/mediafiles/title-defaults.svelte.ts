// Shared loader for the saved add-title defaults, wired to the API.
import { api } from '../core/api.ts';
import { defaultsLoader } from './title-defaults.ts';
import type { TitleDefaults } from './types.ts';

export const savedDefaults = defaultsLoader((p) => api.get<TitleDefaults>(p));
