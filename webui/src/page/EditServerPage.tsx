import { Component, createSignal, Show } from 'solid-js'

import stylesCommon from '../common.module.css'
import { useGlobalState, useRpcClient } from '../ctx'
import { ConnectError } from '@connectrpc/connect'
import { DefaultServerPort } from '../constant'
import { useLocation, useParams } from '@solidjs/router'

const Page: Component = () => {
	const { uuid } = useParams<{ uuid: string }>()
	const state = useGlobalState()
	const client = useRpcClient()

	const server = state.getServerByUuid(uuid)
	if (!server) {
		return <h1>No such server "{uuid}"</h1>
	}

	const [name, setName] = createSignal(server.label())
	const [address, setAddress] = createSignal(server.address())
	const [room, setRoom] = createSignal(server.room())
	const [username, setUsername] = createSignal(server.username())
	const [password, setPassword] = createSignal('')

	const [error, setError] = createSignal('')
	const [isSaving, setSaving] = createSignal(false)
	const [isSuccess, setSuccess] = createSignal(false)
	const submit = async function (event: SubmitEvent) {
		event.preventDefault()

		if (isSaving()) {
			return
		}

		setError('')
		setSuccess(false)
		setSaving(true)

		try {
			if (!name() || !address() || !room() || !username()) {
				setError('Missing params')
				return
			}

			let addr = address()
			if (!addr.includes(':')) {
				addr += ':' + DefaultServerPort
			}

			await server.update(client, {
				name: name(),
				address: addr,
				room: room(),
				username: username(),
				password: password() || undefined,
			})

			setSuccess(true)

			setName(server.label())
			setAddress(server.address())
			setRoom(server.room())
			setUsername(server.username())
			setPassword('')
		} catch (err) {
			if (err instanceof ConnectError) {
				setError(err.message)
			} else {
				console.error('failed to update server:', err)
				setError('Internal error, check console')
			}
		} finally {
			setSaving(false)
		}
	}

	return (
		<div
			classList={{
				[stylesCommon.center]: true,
				[stylesCommon.w100]: true,
			}}
		>
			<Show when={error()}>
				<div class={stylesCommon.errorMessage}>{error()}</div>
			</Show>
			<Show when={isSuccess()}>
				<div class={stylesCommon.successMessage}>Saved</div>
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
								<label for="edit-server-password">
									Password
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
					disabled={isSaving()}
				/>
			</form>
		</div>
	)
}

export const EditServerPage: Component = () => {
	const loc = useLocation()

	return (
		<Show when={loc.pathname} keyed>
			<Page />
		</Show>
	)
}
