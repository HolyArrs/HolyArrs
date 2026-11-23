// The owner reset command shown on the sign-in page and in My account. The
// image has no shell, so /holyarrs stays in it, and -it passes typing through.
export const RESET_CMD = 'docker exec -it holyarrs /holyarrs reset-owner';

// Where each platform shows the container log, for "Forgot password?". These
// are the platforms' own button names, so they stay in English.
export const LOG_PLACES: { name: string; path: string }[] = [
	{ name: 'Docker', path: 'docker logs holyarrs' },
	{ name: 'Docker Desktop', path: 'Containers → holyarrs → Logs' },
	{ name: 'Unraid', path: 'Docker → HolyArrs icon → Logs' },
	{ name: 'TrueNAS', path: 'Apps → HolyArrs → Logs' },
	{ name: 'Synology', path: 'Container Manager → Container → HolyArrs → Log' },
	{ name: 'Portainer', path: 'Containers → HolyArrs → Logs' }
];
