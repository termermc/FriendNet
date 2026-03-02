import styles from './ServerSearchPage.module.css'

import { Component, createSignal, onCleanup, Show } from 'solid-js'
import { useFileServerUrl, useGlobalState, useRpcClient } from '../ctx'
import { Code, ConnectError } from '@connectrpc/connect'
import { A, useLocation, useParams } from '@solidjs/router'
import { FileTable, FileTableItem } from '../FileTable'
import { StreamSearchResponse } from '../../pb/clientrpc/v1/rpc_pb'
import Fuse from 'fuse.js'
import { makeBrowsePath, makeFileUrl } from '../util'

const Page: Component = () => {
	const { uuid } = useParams<{ uuid: string }>()
	const state = useGlobalState()
	const client = useRpcClient()
	const fsUrl = useFileServerUrl()

	const server = state.getServerByUuid(uuid)
	if (!server) {
		return <h1>No such server "{uuid}"</h1>
	}

	const [username, setUsername] = createSignal('')
	const [query, setQuery] = createSignal('')
	let submittedQuery = ''

	const [error, setError] = createSignal('')
	const [isLoading, setLoading] = createSignal(false)

	const [results, setResults] = createSignal<FileTableItem<StreamSearchResponse>[]>([])

	const newItems: FileTableItem<StreamSearchResponse>[] = []
	const debounceInterval = setInterval(() => {
		if (newItems.length === 0) {
			return
		}

		const newRes = [
			...results(),
			...newItems,
		]

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
		setResults(fuse.search(submittedQuery).map(x => x.item))

		newItems.length = 0
	}, 100)

	onCleanup(() => {
		clearInterval(debounceInterval)
	})

	let abortController: AbortController | undefined = undefined

	const submit = async function (event: SubmitEvent) {
		event.preventDefault()

		const q = query()
		if (!q) {
			return
		}

		submittedQuery = q

		abortController?.abort()
		abortController = new AbortController()

		setError('')
		setLoading(true)
		setResults([])

		try {
			const stream = client.streamSearch({
				serverUuid: uuid,
				username: username().trim() || undefined,
				query: q,
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
					const filePath = item.data.directoryPath + '/' + item.meta.name
					const username = item.data.username

					const prefix = (
						<div class={styles.username}>
							👤{username}
						</div>
					)

					if (item.meta.isDir) {
						return {
							prefix: prefix,
							href: makeBrowsePath(uuid, username, filePath),
						}
					} else {
						const dlUrl = makeFileUrl(
							fsUrl,
							uuid,
							username,
							filePath,
							{
								download: true,
							},
						)
						const nonDlUrl = makeFileUrl(
							fsUrl,
							uuid,
							username,
							filePath,
						)

						const dirBrowsePath = makeBrowsePath(uuid, username, item.data.directoryPath)
						console.log(item.data)

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
									<a
										title="Download File"
										href={dlUrl}
									>
										💾
									</a>
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
