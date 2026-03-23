import styles from './TransfersPage.module.css'

import { Component, For, onMount, Show } from 'solid-js'
import { useGlobalState } from '../ctx'

/**
 * The transfers page shows active transfers (uploads and downloads) and allows management of them.
 */
export const TransfersPage: Component = () => {
	const state = useGlobalState()
	const trans = state.transfer

	onMount(() => {
		// Refresh items on page open, in case they're out of date for some reason.
		trans
			.refreshItems()
			.catch((err) => console.error('failed to refresh transfers:', err))
	})

	return (
		<div class={styles.container}>
			<h1>Downloads</h1>

			<Show
				when={trans.downloads().length > 0}
				fallback={<i>No downloads yet.</i>}
			>
				<For each={trans.downloads()}>
					{(item) => <div>{item.filePath}</div>}
				</For>
			</Show>
		</div>
	)
}
