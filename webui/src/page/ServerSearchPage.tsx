import styles from './ServerSearchPage.module.css'

import {
	Component,
	createEffect,
	createSignal,
	onCleanup,
	onMount,
	Show,
} from 'solid-js'
import { useFileServerUrl, useGlobalState, useRpcClient } from '../ctx'
import { Code, ConnectError } from '@connectrpc/connect'
import { A, useLocation, useParams, useSearchParams } from '@solidjs/router'
import { FileTable, FileTableItem } from '../FileTable'
import { StreamSearchResponse } from '../../pb/clientrpc/v1/rpc_pb'
import Fuse from 'fuse.js'
import { makeBrowsePath, makeFileUrl } from '../util'
import { QueueButton } from '../QueueButton'

const Page: Component = () => {
	const { uuid } = useParams<{ uuid: string }>()
	const state = useGlobalState()
	const client = useRpcClient()
	const fsUrl = useFileServerUrl()

	const server = state.getServerByUuid(uuid)
	if (!server) {
		return <h1>No such server "{uuid}"</h1>
	}

	let fieldQueryElem: HTMLInputElement | undefined
	onMount(() => {
		fieldQueryElem?.focus()
	})

	const [searchParams, setSearchParams] = useSearchParams<{
		query?: string
		username?: string
	}>()

	const [query, setQuery] = createSignal(searchParams.query ?? '')
	const [username, setUsername] = createSignal(searchParams.username ?? '')

	const [error, setError] = createSignal('')
	const [isLoading, setLoading] = createSignal(false)

	const [results, setResults] = createSignal<
		FileTableItem<StreamSearchResponse>[]
	>([])

	const maxItems = 1_000
	const newItems: FileTableItem<StreamSearchResponse>[] = []
	const debounceInterval = setInterval(() => {
		const q = searchParams.query
		if (!q) {
			return
		}

		if (newItems.length === 0) {
			return
		}

		const newRes = [...results(), ...newItems]

		// Sort with Fuse.
		newItems[0].data.directoryPath
		const fuse = new Fuse(newRes, {
			keys: [
				{
					name: 'meta.name',
					weight: 2,
				},
				{
					name: 'data.directoryPath',
					weight: 1,
				},
			],
		})

		// Remove "ext:" filters from search term before using it with Fuse.
		const fuzzyQ = q.replace(/(^|\W)ext:\w*/g, ' ')

		// Truncate results to limit.
		const fuzzyRes = fuse.search(fuzzyQ)
		if (fuzzyRes.length > maxItems) {
			fuzzyRes.length = maxItems
		}

		setResults(fuzzyRes.map((x) => x.item))

		newItems.length = 0
	}, 100)

	onCleanup(() => {
		clearInterval(debounceInterval)
	})

	let abortController: AbortController | undefined = undefined

	const submit = async function (event: SubmitEvent) {
		event.preventDefault()

		setSearchParams({ query: query().trim(), username: username().trim() })
	}

	async function doSearch(query: string, username: string) {
		abortController?.abort()
		abortController = new AbortController()

		setError('')
		setLoading(true)
		setResults([])

		try {
			const stream = client.streamSearch({
				serverUuid: uuid,
				username: username || undefined,
				query: query,
			})

			for await (const res of stream) {
				newItems.push({
					meta: res.file!,
					data: res,
				})
			}
		} catch (err) {
			if (err instanceof ConnectError) {
				if (err.code === Code.Canceled) {
					return
				}

				setError(err.message)
			} else {
				console.error('failed to stream search results:', err)
				setError('Internal error, check console')
			}
		} finally {
			setLoading(false)
		}
	}

	createEffect(() => {
		const q = searchParams.query?.trim() || ''
		const u = searchParams.username?.trim() || ''

		fieldQueryElem?.focus()

		if (!q) {
			setResults([])
			setQuery('')
			setUsername('')
			return
		}

		setQuery(q)
		setUsername(u)

		// noinspection JSIgnoredPromiseFromCall
		doSearch(q, u)
	})

	return (
		<div class={styles.container}>
			<form class={styles.form} onSubmit={submit}>
				<input
					class={styles.fieldUsername}
					type="text"
					placeholder="Optional Username"
					value={username()}
					onChange={(e) => setUsername(e.currentTarget.value)}
				/>

				<input
					ref={fieldQueryElem}
					class={styles.fieldQuery}
					type="text"
					placeholder="Search Query"
					value={query()}
					onChange={(e) => setQuery(e.currentTarget.value)}
				/>

				<input
					class={styles.fieldSubmit}
					type="submit"
					placeholder="Search"
				/>
			</form>

			<FileTable
				isLoading={isLoading()}
				error={error()}
				items={results()}
				forItem={(item) => {
					const filePath =
						item.data.directoryPath + '/' + item.meta.name
					const username = item.data.username

					const prefix = (
						<div class={styles.username}>👤{username}</div>
					)

					if (item.meta.isDir) {
						return {
							prefix: prefix,
							href: makeBrowsePath(uuid, username, filePath),
						}
					} else {
						const nonDlUrl = makeFileUrl(
							fsUrl,
							uuid,
							username,
							filePath,
						)

						const dirBrowsePath = makeBrowsePath(
							uuid,
							username,
							item.data.directoryPath,
						)

						return {
							prefix: prefix,
							actions: (
								<>
									<A
										title="Open Directory"
										href={dirBrowsePath}
									>
										📁
									</A>
									<a
										title="Open File"
										href={nonDlUrl}
										target="_blank"
									>
										🔗
									</a>
									<QueueButton
										serverUuid={uuid}
										peerUsername={username}
										filePath={filePath}

										title="Download File"
									/>
								</>
							),
							onClick: () => {
								state.previewFile(uuid, username, filePath)
							},
						}
					}
				}}
			/>
		</div>
	)
}

export const ServerSearchPage: Component = () => {
	const loc = useLocation()

	return (
		<Show when={loc.pathname} keyed>
			<Page />
		</Show>
	)
}
