import { Component, createSignal, For, onMount, Show } from 'solid-js'

import styles from './ServerSharesPage.module.css'
import stylesCommon from '../common.module.css'

import { useGlobalState, useRpcClient } from '../ctx'
import { ConnectError } from '@connectrpc/connect'
import { useLocation, useParams } from '@solidjs/router'

const Page: Component = () => {
	const { uuid } = useParams<{ uuid: string }>()
	const state = useGlobalState()
	const client = useRpcClient()

	const server = state.getServerByUuid(uuid)
	if (!server) {
		return <h1>No such server "{uuid}"</h1>
	}

	onMount(() => {
		server.refreshShares(client).catch((err) => {
			console.error('failed to refresh shares:', err)
			alert('Failed to refresh shares, check console')
		})
	})

	const [name, setName] = createSignal('')
	const [path, setPath] = createSignal('')

	const [error, setError] = createSignal('')
	const [isAdding, setAdding] = createSignal(false)
	const [isSuccess, setSuccess] = createSignal(false)
	const submit = async function (event: SubmitEvent) {
		event.preventDefault()

		if (isAdding()) {
			return
		}

		setError('')
		setSuccess(false)
		setAdding(true)

		try {
			if (!name() || !path()) {
				setError('Missing params')
				return
			}

			await server.createShare(client, {
				name: name(),
				path: path(),
			})

			setSuccess(true)

			setName('')
			setPath('')
		} catch (err) {
			if (err instanceof ConnectError) {
				setError(err.message)
			} else {
				console.error('failed to create share:', err)
				setError('Internal error, check console')
			}
		} finally {
			setAdding(false)
		}
	}

	const deletingNames = new Set<string>()
	const doDelete = async (name: string) => {
		if (deletingNames.has(name)) {
			return
		}

		if (!confirm('Are you sure?')) {
			return
		}

		deletingNames.add(name)
		try {
			await server.deleteShare(client, name)
		} finally {
			deletingNames.delete(name)
		}
	}

	return (
		<div
			classList={{
				[stylesCommon.center]: true,
				[stylesCommon.w100]: true,
			}}
		>
			<h1>Shares for {server.name()}</h1>

			<div class={styles.shares}>
				<Show
					when={server.shares().length > 0}
					fallback={<p>No shares</p>}
				>
					<For each={server.shares()}>
						{(share) => (
							<div class={styles.share}>
								Name: {share.name}
								<br />
								Path: {share.path}
								<br />
								<button onClick={() => doDelete(share.name)}>
									Delete
								</button>
							</div>
						)}
					</For>
				</Show>
			</div>

			<Show when={error()}>
				<div class={stylesCommon.errorMessage}>{error()}</div>
			</Show>
			<Show when={isSuccess()}>
				<div class={stylesCommon.successMessage}>Added</div>
			</Show>

			<h1>Add Share</h1>

			<form onSubmit={submit} class={stylesCommon.form}>
				<table>
					<tbody>
						<tr>
							<td>
								<label for="add-share-name">Name</label>
							</td>
							<td>
								<input
									id="add-share-name"
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
								<label for="add-share-path">Local Path</label>
							</td>
							<td>
								<input
									id="add-share-path"
									type="text"
									placeholder="/mnt/Music, D:\Music, etc."
									value={path()}
									onChange={(e) =>
										setPath(e.currentTarget.value)
									}
									required={true}
								/>
							</td>
						</tr>
					</tbody>
				</table>

				<input type="submit" value="Add Share" disabled={isAdding()} />
			</form>
		</div>
	)
}

export const ServerSharesPage: Component = () => {
	const loc = useLocation()

	return (
		<Show when={loc.pathname} keyed>
			<Page />
		</Show>
	)
}
