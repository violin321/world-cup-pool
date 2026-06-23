import { pb } from './pb';

export type PublicConfig = {
	googleOAuthEnabled: boolean;
};

const defaultConfig: PublicConfig = {
	googleOAuthEnabled: false
};

class PublicConfigState {
	value = $state<PublicConfig>(defaultConfig);
	loaded = $state(false);
	private loading: Promise<void> | null = null;

	load() {
		if (this.loading) return this.loading;
		this.loading = pb
			.send<Partial<PublicConfig>>('/api/public/config', { method: 'GET' })
			.then((config) => {
				this.value = {
					googleOAuthEnabled: config.googleOAuthEnabled === true
				};
			})
			.catch(() => {
				this.value = defaultConfig;
			})
			.finally(() => {
				this.loaded = true;
				this.loading = null;
			});
		return this.loading;
	}
}

export const publicConfig = new PublicConfigState();
