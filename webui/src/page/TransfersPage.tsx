import styles from './TransfersPage.module.css'

import { Component, For, onMount, Show } from 'solid-js'
import { useGlobalState } from '../ctx'
import { formatSize, formatSpeed } from '../util'

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
					{(item) => <div class={styles.transfer}>
						<div class={styles.info}>{item.filePath}</div>
						<div class={styles.progress}>
							<progress value={item.downloadedBytes() / item.fileSizeBytes()} max="1" />
							<div>
								{formatSize(item.downloadedBytes(), 2)} / {formatSize(item.fileSizeBytes(), 2)}
								{' | '}
								{formatSpeed(item.lastSpeedBytesPerSecond())}
							</div>
						</div>
					</div>}
				</For>
			</Show>
		</div>
	)
}
