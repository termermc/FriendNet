import styles from './ServerSearchPage.module.css'
import stylesCommon from '../common.module.css'

import { Component, createSignal, onCleanup, Show } from 'solid-js'
import { useGlobalState, useRpcClient } from '../ctx'
import { Code, ConnectError } from '@connectrpc/connect'
import { useLocation, useParams } from '@solidjs/router'
import { FileTableItem } from '../FileTable'
import { StreamSearchResponse } from '../../pb/clientrpc/v1/rpc_pb'
import Fuse from 'fuse.js'

const Page: Component = () => {
	const { uuid } = useParams<{ uuid: string }>()
	const state = useGlobalState()
	const client = useRpcClient()

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
			<Show when={error()}>
				<div class={stylesCommon.errorMessage}>{error()}</div>
			</Show>

			<h1>Edit Server</h1>

			<form onSubmit={submit} class={stylesCommon.form}>
				<table>
					<tbody>
						<tr>
							<td>
								<label for="edit-server-name">Name</label>
							</td>
							<td>
								<input
									id="edit-server-name"
									type="text"
									placeholder=""
									value={name()}
									onChange={(e) =>
										setName(e.currentTarget.value)
									}
									required={true}
								/>
							</td>
						</tr>

						<tr>
							<td>
								<label for="edit-server-address">Address</label>
							</td>
							<td>
								<input
									id="edit-server-address"
									type="text"
									placeholder="example.com, example.com:20038, etc."
									value={address()}
									onChange={(e) =>
										setAddress(e.currentTarget.value)
									}
									required={true}
								/>
							</td>
						</tr>

						<tr>
							<td>
								<label for="edit-server-room">Room</label>
							</td>
							<td>
								<input
									id="edit-server-room"
									type="text"
									placeholder=""
									value={room()}
									onChange={(e) =>
										setRoom(e.currentTarget.value)
									}
									required={true}
								/>
							</td>
						</tr>

						<tr>
							<td>
								<label for="edit-server-username">
									Username
								</label>
							</td>
							<td>
								<input
									id="edit-server-username"
									type="text"
									placeholder=""
									value={username()}
									onChange={(e) =>
										setUsername(e.currentTarget.value)
									}
									required={true}
								/>
							</td>
						</tr>

						<tr>
							<td>
								<label
									for="edit-server-password"
									style="cursor:help"
									title='This is the password to log in with. To change your account password, click "Change Account Password" on the server in the server browser.'
								>
									Password<sup>🛈</sup>
								</label>
							</td>
							<td>
								<input
									id="edit-server-password"
									type="password"
									placeholder="Leave blank to leave unchanged"
									value={password()}
									onChange={(e) =>
										setPassword(e.currentTarget.value)
									}
								/>
							</td>
						</tr>
					</tbody>
				</table>

				<input
					type="submit"
					value="Save Changes"
					disabled={isLoading()}
				/>
			</form>
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
