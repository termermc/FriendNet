import { State } from './state'

export class Download {
	// TODO Signals for the variable parts, normal properties for the download constants
}

export class TransferManager {
	#state: State

	constructor(state: State) {
		this.#state = state

		// TODO Listen for event bus download manager updates.
	}
}
