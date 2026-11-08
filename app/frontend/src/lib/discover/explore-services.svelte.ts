// Explore's streaming service picker: the person's services and the choice kept for later.
import { defaultRegion } from '#lib/ui/region.ts';
import { pickerServices, readServices, serviceRegion, SERVICES_KEY, SERVICES_SHOWN, type WatchService } from './explore-services.ts';
import { api } from '#lib/core/api.ts';

export function createExploreServices(ctx: any) {
	const region = serviceRegion(defaultRegion());
	let services = $state<WatchService[]>([]);
	/** null until the person picks once (the row then asks them to). */
	let picked = $state<number[] | null>(null);
	const shownServices = $derived(pickerServices(services, picked ?? []));
	/** The ids the row is filled from: the picked ones, else the region's top few. */
	const usedServices = $derived(picked ?? shownServices.slice(0, SERVICES_SHOWN).map((s) => s.id));

	function readPicked() {
		try {
			picked = readServices(region, localStorage.getItem(SERVICES_KEY));
		} catch {
			/* Nothing saved. */
		}
	}

	async function loadServices() {
		try {
			services = (await api.get<{ services: WatchService[] }>(`/watch-services?region=${region}`, ctx.gone.signal)).services ?? [];
		} catch {
			services = [];
		}
	}

	// Leaving Explore stops the TMDB-backed asks still on their way.

	return {
		get picked() {
			return picked;
		},
		set picked(v) {
			picked = v;
		},
		get region() {
			return region;
		},
		readPicked,
		loadServices,
		get services() {
			return services;
		},
		set services(v) {
			services = v;
		},
		get usedServices() {
			return usedServices;
		},
	};
}
