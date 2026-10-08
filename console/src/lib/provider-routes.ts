/** Where an old link lands now that Models lives inside the provider that serves it.
 *  A link that named a provider keeps it and opens its models tab; anything else opens the
 *  all-models view. */
export function modelsRedirectTarget(search: string): string {
	const provider = new URLSearchParams(search).get('provider');
	if (provider !== null && provider.trim() !== '') {
		return '/connections?provider=' + encodeURIComponent(provider) + '&tab=models';
	}
	return '/connections?view=models';
}
