import styles from './SearchPage.module.css'

import {
	Component,
	createEffect,
	createSignal,
	onCleanup,
	onMount,
	Show,
	For,
  JSX,
} from 'solid-js'
import { useFileServerUrl, useGlobalState, useRpcClient } from '../ctx'
import { Code, ConnectError } from '@connectrpc/connect'
import { A, useLocation, useSearchParams } from '@solidjs/router'
import { FileTable, FileTableItem } from '../FileTable'
import { StreamSearchResponse } from '../../pb/clientrpc/v1/rpc_pb'
import Fuse from 'fuse.js'
import { makeBrowsePath, makeFileUrl } from '../util'
import { QueueButton } from '../QueueButton'

const Page: Component = () => {
	const state = useGlobalState()
	const client = useRpcClient()
	const fsUrl = useFileServerUrl()

	const [searchParams, setSearchParams] = useSearchParams<{
		server?: string
		query?: string
		username?: string
	}>()

	onMount(() => {
		fieldQueryElem?.focus()
	})

	let fieldQueryElem: HTMLInputElement | undefined

  const [query, setQuery] = createSignal(searchParams.query ?? '')
	const [serverUuid, setServerUuid] = createSignal(searchParams.server ?? '')
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

	const [lastSearch, setLastSearch] = createSignal(Date.now())
	const submit = async function (event: SubmitEvent) {
		event.preventDefault()

    setSearchParams({ query: query().trim(), server: serverUuid() || null, username: username().trim() || null })
		setLastSearch(Date.now())
	}

	async function doSearch(query: string, serverUuid: string | undefined, username: string) {
		abortController?.abort()
    abortController = new AbortController()

		setError('')
		setLoading(true)
		setResults([])

    try {
      // Check if the server actually exists if the UUID is set.
  		if (serverUuid && state.getServerByUuid(serverUuid) == null) {
        setError(`No such server UUID "${serverUuid}"`)
        return
      }

			const stream = client.streamSearch({
				serverUuid: serverUuid || undefined,
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
    // Re-run everytime we press the search button, even if params didn't change.
    lastSearch()

    const q = searchParams.query?.trim() || ''
		const s = searchParams.server?.trim() || ''
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
    setServerUuid(s)

		// noinspection JSIgnoredPromiseFromCall
		doSearch(q, s, u)
	})

	return (
				<div class={styles.container}>
					<form class={styles.form} onSubmit={submit}>
						<select
							value={serverUuid()}
							onChange={(e) =>
								setSearchParams({
									server: e.currentTarget.value || '',
								})
							}
						>
							<option value="">All Servers</option>
							<For each={state.servers()}>
								{(srv) => (
									<option value={srv.uuid}>
										{srv.name()}
									</option>
								)}
							</For>
						</select>

						<input
							class={styles.fieldUsername}
							type="text"
							placeholder="Optional Username"
							value={username()}
							onChange={(e) =>
								setUsername(e.currentTarget.value)
							}
              disabled={!serverUuid()}
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
						const srvUuid = item.data.serverUuid
							const filePath =
								item.data.directoryPath + '/' + item.meta.name
              const username = item.data.username

              let prefix: JSX.Element
              if (serverUuid()) {
                prefix = (
                  <div class={styles.username}>👤{username}</div>
                )
              } else {
                // termer 2026/07/29: This is inefficient since it does a server lookup for every result.
                // If this becomes actually slow, I'll make a different forItem implementation based on
                // whether this is a global or server search.
                // Oh well.
                const srv = state.getServerByUuid(srvUuid)
                prefix = (
                  <div class={styles.username}>👤{username} @ {srv?.name() ?? 'Unknown'}</div>
                )
              }

							if (item.meta.isDir) {
								return {
									prefix: prefix,
									href: makeBrowsePath(
										item.data.serverUuid,
										username,
										filePath,
									),
									actions: (
										<QueueButton
											serverUuid={srvUuid}
											peerUsername={username}
											filePath={filePath}
											title="Download Folder"
										/>
									),
								}
							} else {
								const nonDlUrl = makeFileUrl(
									fsUrl,
									srvUuid,
									username,
									filePath,
								)

								const dirBrowsePath = makeBrowsePath(
									srvUuid,
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
												serverUuid={srvUuid}
												peerUsername={username}
												filePath={filePath}
												title="Download File"
											/>
										</>
									),
									onClick: () => {
										state.previewFile(
											srvUuid,
											username,
											filePath,
										)
									},
								}
							}
						}}
					/>
				</div>
	)
}

export const SearchPage: Component = () => {
	const loc = useLocation()

	return (
		<Show when={loc.pathname} keyed>
			<Page />
		</Show>
	)
}
