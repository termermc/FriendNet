import styles from './TransfersPage.module.css'

import { Component, createMemo, JSX, onMount, Show } from 'solid-js'
import { useGlobalState } from '../ctx'
import { formatSize, formatSpeed, makeBrowsePath } from '../util'
import { Download } from '../transfer'
import { A } from '@solidjs/router'

const DownloadItem: Component<{ item: Download }> = (props) => {
	const filename = props.item.filePath.substring(props.item.filePath.lastIndexOf('/') + 1)

	return (
		<div class={styles.transfer}>
			<div class={styles.info}>{filename}</div>
			<div class={styles.progress}>
				<progress value={props.item.downloadedBytes() / props.item.fileSizeBytes()} max="1"/>
				<div>
					{formatSize(props.item.downloadedBytes(), 2)}
					{' / '}
					{props.item.fileSizeBytes() === -1 ? '???' : formatSize(props.item.fileSizeBytes(), 2)}
					{' | '}
					{formatSpeed(props.item.lastSpeedBytesPerSecond())}
				</div>
			</div>
		</div>
	)
}

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

	const sortedItems = createMemo(() => {
		const items = trans.downloads()

		// Sort items based on server, peer, and file path.
		items.sort((a, b) => {
			const strA = `${a.server.uuid}:${a.peerUsername}:${a.filePath}`
			const strB = `${b.server.uuid}:${b.peerUsername}:${b.filePath}`
			return strA.localeCompare(strB)
		})

		return items
	})

	const markup = createMemo(() => {
		const elems: JSX.Element[] = []

		type Container = {
			server: Download['server']
			peerUsername: Download['peerUsername']
			containingDir: string
			items: JSX.Element[]
		}
		let lastContainer: Container | null = null

		const flushContainer = () => {
			if (lastContainer != null && lastContainer.items.length > 0) {
				elems.push(
					<div class={styles.itemContainer}>
						<A
							href={makeBrowsePath(
								lastContainer.server.uuid,
								lastContainer.peerUsername,
								'/',
							)}
							class={styles.peer}
						>
							{'👤 '}
							{lastContainer.peerUsername}
							{' @ '}
							{lastContainer.server.name()}
						</A>
						<A
							href={makeBrowsePath(
								lastContainer.server.uuid,
								lastContainer.peerUsername,
								lastContainer.containingDir,
							)}
							class={styles.path}
						>
							{'📁 '}
							{lastContainer.containingDir}
						</A>

						<div class={styles.items}>
							{lastContainer.items}
						</div>
					</div>
				)
			}
		}

		const items = sortedItems()
		for (let i = 0; i < items.length; i++) {
			let lastItem = i === 0 ? null : items[i - 1]
			let item = items[i]

			const serverUuid = item.server.uuid
			const peerUsername = item.peerUsername
			const filePath = item.filePath
			const containingDir = filePath.substring(0, filePath.lastIndexOf('/'))

			const hash = `${serverUuid}:${peerUsername}:${containingDir}`
			let lastHash: string | null = null

			if (lastItem != null) {
				lastHash = `${lastItem.server.uuid}:${lastItem.peerUsername}:${lastItem.filePath.substring(0, lastItem.filePath.lastIndexOf('/'))}`
			}

			if (lastContainer == null || hash !== lastHash) {
				flushContainer()
				lastContainer = {
					server: item.server,
					peerUsername: item.peerUsername,
					containingDir: containingDir,
					items: [],
				}
			}

			lastContainer.items.push(<DownloadItem item={item}/>)
		}

		flushContainer()

		return elems
	})

	return (
		<div class={styles.container}>
			<h1>Downloads</h1>

			<Show
				when={sortedItems().length > 0}
				fallback={<i>No downloads yet.</i>}
			>
				{markup()}
			</Show>
		</div>
	)
}
