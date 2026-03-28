import styles from './TransfersPage.module.css'

import {
	Component,
	createMemo,
	For,
	JSX,
	Match,
	onMount,
	Show,
	Switch,
} from 'solid-js'
import { useGlobalState, useRpcClient } from '../ctx'
import { formatSize, formatSpeed, makeBrowsePath } from '../util'
import { Download } from '../transfer'
import { A } from '@solidjs/router'
import { DownloadStatus } from '../../pb/clientrpc/v1/rpc_pb'
import { RpcClient } from '../protobuf'
import { Code, ConnectError } from '@connectrpc/connect'

async function doRemove(client: RpcClient, uuid: string): Promise<void> {
	try {
		await client.removeDownloadManagerItem({ uuid })
	} catch (err) {
		if (err instanceof ConnectError) {
			if (err.code !== Code.NotFound) {
				console.error('failed to remove item:', err)
				alert('Failed to remove item: ' + err.message)
				return
			}
		} else {
			console.error('failed to remove item:', err)
			alert('Failed to remove item, check console for details')
			return
		}
	}
}
async function doCancel(client: RpcClient, uuid: string): Promise<void> {
	try {
		await client.cancelFileDownload({ uuid })
	} catch (err) {
		console.error('failed to cancel download:', err)
		alert('Failed to cancel download, check console for details')
	}
}

const DownloadFolder: Component<{
	server: Download['server']
	peerUsername: Download['peerUsername']
	containingDir: string
	items: Download[]
}> = (props) => {
	const client = useRpcClient()
	const items = props.items

	async function removeAll() {
		for (const item of items) {
			await doRemove(client, item.uuid)
		}
	}

	return (
		<div class={styles.itemContainer}>
			<button
				onClick={removeAll}
				title="Remove (does not remove files on disk)"
			>
				🗑️
			</button>{' '}
			<A
				href={makeBrowsePath(
					props.server.uuid,
					props.peerUsername,
					'/',
				)}
				class={styles.peer}
			>
				{'👤 '}
				{props.peerUsername}
				{' @ '}
				{props.server.name()}
			</A>
			<A
				href={makeBrowsePath(
					props.server.uuid,
					props.peerUsername,
					props.containingDir,
				)}
				class={styles.path}
			>
				{'📁 '}
				{props.containingDir}
			</A>
			<div class={styles.items}>
				<For each={items}>{(item) => <DownloadItem item={item} />}</For>
			</div>
		</div>
	)
}

const DownloadItem: Component<{ item: Download }> = (props) => {
	const client = useRpcClient()

	const item = props.item
	const filename = item.filePath.substring(item.filePath.lastIndexOf('/') + 1)

	async function doResume() {
		// TODO This does NOT resume the file, it RESTARTS it.
		// Make an actual resume/download now function.

		await client.queueFileDownload({
			serverUuid: item.server.uuid,
			peerUsername: item.peerUsername,
			filePath: item.filePath,
		})
	}

	return (
		<div
			classList={{
				[styles.transfer]: true,
				[styles.canceled]: item.status() === DownloadStatus.CANCELED,
				[styles.done]: item.status() === DownloadStatus.DONE,
				[styles.pending]: item.status() === DownloadStatus.PENDING,
				[styles.queued]: item.status() === DownloadStatus.QUEUED,
				[styles.error]: item.status() === DownloadStatus.ERROR,
			}}
		>
			<div class={styles.info}>{filename}</div>
			<div class={styles.progress}>
				<progress
					value={item.downloadedBytes() / item.fileSizeBytes()}
					max="1"
				/>
				<div class={styles.options}>
					<button
						onClick={() => doRemove(client, item.uuid)}
						title="Remove (does not remove files on disk)"
					>
						🗑️
					</button>{' '}
					<Switch>
						<Match when={item.status() === DownloadStatus.CANCELED}>
							<button onClick={doResume} title="Resume">
								⏩
							</button>
						</Match>
						<Match when={item.status() === DownloadStatus.DONE}>
							<b>Done</b>
						</Match>
						<Match when={item.status() === DownloadStatus.PENDING}>
							<button
								onClick={() => doCancel(client, item.uuid)}
								title="Cancel"
							>
								⛔
							</button>
						</Match>
						<Match when={item.status() === DownloadStatus.QUEUED}>
							<button title="Download Now">➡️</button>
						</Match>
						<Match when={item.status() === DownloadStatus.ERROR}>
							<Match
								when={item.status() === DownloadStatus.CANCELED}
							>
								<button title="Retry">🔄</button>
							</Match>
							<span class={styles.errorMessage}>
								Error: {item.errorMessage()}
							</span>
						</Match>
					</Switch>
				</div>
				{' | '}
				<div class={styles.stats}>
					{formatSize(item.downloadedBytes(), 2)}
					{' / '}
					{item.fileSizeBytes() === -1
						? '???'
						: formatSize(item.fileSizeBytes(), 2)}
					<Show when={item.status() !== DownloadStatus.DONE}>
						{' | '}
						{formatSpeed(item.lastSpeedBytesPerSecond())}
					</Show>
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
			items: Download[]
		}
		let lastContainer: Container | null = null

		const flushContainer = () => {
			if (lastContainer != null && lastContainer.items.length > 0) {
				elems.push(
					<DownloadFolder
						server={lastContainer.server}
						peerUsername={lastContainer.peerUsername}
						containingDir={lastContainer.containingDir}
						items={lastContainer.items}
					/>,
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
			const containingDir = filePath.substring(
				0,
				filePath.lastIndexOf('/'),
			)

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

			lastContainer.items.push(item)
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
