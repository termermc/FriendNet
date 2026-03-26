import { Server, State } from './state'
import { Accessor, createSignal, Setter } from 'solid-js'
import {
	DownloadManagerItem,
	DownloadManagerItem_Type,
	DownloadStatus,
	DownloadStatusUpdate,
	Event_Type,
} from '../pb/clientrpc/v1/rpc_pb'
import { RpcClient } from './protobuf'

export class Download {
	readonly uuid: string
	readonly server: Server
	readonly peerUsername: string
	readonly filePath: string

	readonly status: Accessor<DownloadStatus>
	readonly #setStatus: Setter<DownloadStatus>
	readonly downloadedBytes: Accessor<number>
	readonly #setDownloadedBytes: Setter<number>
	readonly fileSizeBytes: Accessor<number | -1>
	readonly #setFileSizeBytes: Setter<number | -1>
	readonly errorMessage: Accessor<string | undefined>
	readonly #setErrorMessage: Setter<string | undefined>
	readonly lastSpeedBytesPerSecond: Accessor<number>
	readonly #setLastSpeedBytesPerSecond: Setter<number>

	constructor(state: State, item: DownloadManagerItem) {
		const server = state.getServerByUuid(item.serverUuid)

		if (server == null) {
			throw new Error(
				`tried to construct Download with server UUID for a server that is not known: ${item.serverUuid}`,
			)
		}

		this.uuid = item.uuid
		this.server = server
		this.peerUsername = item.peerUsername
		this.filePath = item.filePath
		;[this.status, this.#setStatus] = createSignal<DownloadStatus>(
			DownloadStatus.UNSPECIFIED,
		)
		;[this.downloadedBytes, this.#setDownloadedBytes] = createSignal(0)
		;[this.fileSizeBytes, this.#setFileSizeBytes] = createSignal(0)
		;[this.errorMessage, this.#setErrorMessage] = createSignal()
		;[this.lastSpeedBytesPerSecond, this.#setLastSpeedBytesPerSecond] =
			createSignal(0)

		this.updateFromItem(item)
	}

	updateFromUpdate(update: DownloadStatusUpdate) {
		this.#setStatus(update.status)
		this.#setDownloadedBytes(Number(update.downloaded))
		this.#setFileSizeBytes(Number(update.fileSize))
		this.#setErrorMessage(update.errorMessage)
		this.#setLastSpeedBytesPerSecond(Number(update.speed))
	}
	updateFromItem(item: DownloadManagerItem) {
		if (item.type !== DownloadManagerItem_Type.DOWNLOAD) {
			throw new Error(
				`tried to construct Download with non-download item: ${item.type}`,
			)
		}

		const dl = item.download!

		this.#setStatus(dl.status)
		this.#setDownloadedBytes(Number(dl.downloaded))
		this.#setFileSizeBytes(Number(dl.fileSize))
		this.#setErrorMessage(dl.errorMessage)
		this.#setLastSpeedBytesPerSecond(0)
	}
}

/**
 * TransferManager manages all transfers (downloads and uploads).
 */
export class TransferManager {
	readonly #state: State
	readonly #client: RpcClient

	readonly downloads: Accessor<Download[]>
	readonly #setDownloads: Setter<Download[]>

	constructor(state: State, client: RpcClient) {
		this.#state = state
		this.#client = client
		;[this.downloads, this.#setDownloads] = createSignal<Download[]>([])

		// Listen for event bus download manager updates.
		this.#state.event.addEventListener(
			Event_Type.DOWNLOAD_STATUS_UPDATES,
			(event) => {
				const files = event.downloadStatusUpdates!.files
				const downloads = this.downloads()
				for (const upd of files) {
					const dl = downloads.find((x) => x.uuid === upd.uuid)
					if (dl == null) {
						continue
					}

					dl.updateFromUpdate(upd)
				}
			},
		)
		this.#state.event.addEventListener(Event_Type.NEW_DM_ITEM, (event) => {
			const item = event.newDmItem!.item!
			this.#setDownloads([
				...this.downloads(),
				new Download(this.#state, item),
			])
		})
	}

	/**
	 * Refreshes the download manager items.
	 */
	async refreshItems(): Promise<void> {
		const { items } = await this.#client.getDownloadManagerItems({})

		const curDownloads = this.downloads()
		const newDownloads: Download[] = []

		for (const item of items) {
			if (item.type !== DownloadManagerItem_Type.DOWNLOAD) {
				continue
			}

			const cur = curDownloads.find((x) => x.uuid === item.uuid)
			if (cur) {
				cur.updateFromItem(item)
				newDownloads.push(cur)
			} else {
				newDownloads.push(new Download(this.#state, item))
			}
		}

		this.#setDownloads(newDownloads)
	}

	/**
	 * Queues a file download.
	 * @param serverUuid The UUID of the server the peer is on.
	 * @param peerUsername The peer's username.
	 * @param filePath The file path within the peer.
	 */
	async queue(serverUuid: string, peerUsername: string, filePath: string): Promise<void> {
		await this.#client.queueFileDownload({
			serverUuid,
			peerUsername,
			filePath,
		})
	}
}
