/** Lists the lanes whose behaviour is still settling, which is what an Experimental chip on a
 *  connection says. The keyless Kilo pool logs every prompt at the vendor, so it is offered for
 *  throwaway work; OpenCode Zen Free is keyless too but logs nothing, and stays unchipped. */
const EXPERIMENTAL_IDS = new Set(['kilo-free']);

/** Reports whether one connection is an experimental lane. */
export function isExperimentalConnection(id: string): boolean {
	return EXPERIMENTAL_IDS.has(id);
}
