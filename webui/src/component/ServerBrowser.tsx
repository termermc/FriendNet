import { Component, createSignal, For, onCleanup, onMount } from 'solid-js'
import { useGlobalState, useRpcClient } from '../ctx'

import styles from './ServerBrowser.module.css'
import { Server, OnlineUser } from '../state'
import { A } from '@solidjs/router'

const OnlineUserEntry: Component<{ server: Server; user: OnlineUser }> = (
	props,
) => {
	return (
		<div class={styles.onlineUser}>
			<div class={styles.onlineUserName}>
				<div class={styles.onlineUserStatus} />
				<span>{props.user.username}</span>
			</div>
			<div class={styles.onlineUserOptions}>
				<A
					href={`/server/${props.server.uuid}/browse/${props.user.username}`}
				>
					📂 Browse
				</A>
				<A
					href={`/server/${props.server.uuid}/profile/${props.user.username}`}
				>
					👤 Profile
				</A>
			</div>
		</div>
	)
}

const ServerEntry: Component<{ server: Server }> = (props) => {
	const state = useGlobalState()
	const client = useRpcClient()

	const refreshUsers = () => {
		props.server.refreshOnlineUsers(client).catch((err) => {
			console.error('failed to refresh online users:', err)
			alert('Failed to refresh online users, see console for details')
		})
	}

	onMount(() => {
		refreshUsers()
	})

	const userRefresher = setInterval(() => {
		refreshUsers()
	}, 1_000)
	onCleanup(() => clearInterval(userRefresher))

	const [isDeleting, setDeleting] = createSignal(false)
	const doDelete = async () => {
		if (isDeleting()) {
			return
		}

		if (
			!confirm(
				`Are you sure you want to delete ${JSON.stringify(props.server.label())}?`,
			)
		) {
			return
		}

		try {
			setDeleting(true)
			await state.deleteServer(client, props.server.uuid)
		} catch (err) {
			console.error(
				`failed to delete server ${JSON.stringify(props.server.uuid)}:`,
				err,
			)
			alert(
				`Failed to delete server ${props.server.uuid}, see console for details`,
			)
		} finally {
			setDeleting(false)
		}
	}

	return (
		<details open={true} class={styles.server}>
			<summary>
				{props.server.label()}

				<A
					title="Edit Server"
					class={styles.action}
					href={`/server/${props.server.uuid}/edit`}
				>
					📝️
				</A>
				<button
					title="Delete Server"
					onClick={doDelete}
					disabled={isDeleting()}
					class={styles.action}
				>
					🗑️
				</button>
			</summary>

			<div class={styles.serverContent}>
				<div class={styles.serverInfo}>
					<table>
						<tbody>
							<tr>
								<td>Address:</td>
								<td>{props.server.address()}</td>
							</tr>
							<tr>
								<td>Room:</td>
								<td>{props.server.room()}</td>
							</tr>
							<tr>
								<td>Username:</td>
								<td>{props.server.username()}</td>
							</tr>
						</tbody>
					</table>
				</div>

				<div class={styles.onlineUsers}>
					<For each={props.server.onlineUsers()}>
						{(user) => (
							<OnlineUserEntry
								server={props.server}
								user={user}
							/>
						)}
					</For>
				</div>
			</div>
		</details>
	)
}

export const ServerBrowser: Component = () => {
	const state = useGlobalState()
	const client = useRpcClient()

	onMount(() => {
		state.refreshServers(client).catch((err) => {
			console.error('failed to refresh servers:', err)
			alert('Failed to refresh servers, see console for details')
		})
	})

	return (
		<div class={styles.container}>
			<details open={true}>
				<summary>
					Servers

					<A
						title="Create New Server"
						class={styles.action}
						href="/createserver"
					>
						➕️
					</A>
				</summary>

				<For each={state.servers()}>
					{(server) => <ServerEntry server={server} />}
				</For>
			</details>
		</div>
	)
}
