import { Component, createSignal, Show } from 'solid-js'

import stylesCommon from '../common.module.css'
import { useGlobalState, useRpcClient } from '../ctx'
import { ConnectError } from '@connectrpc/connect'
import { DefaultServerPort } from '../constant'

export const CreateServerPage: Component = () => {
	const state = useGlobalState()
	const client = useRpcClient()

	const [name, setName] = createSignal('')
	const [address, setAddress] = createSignal('')
	const [room, setRoom] = createSignal('')
	const [username, setUsername] = createSignal('')
	const [password, setPassword] = createSignal('')

	const [error, setError] = createSignal('')
	const [isCreating, setCreating] = createSignal(false)
	const [isSuccess, setSuccess] = createSignal(false)
	const submit = async function (event: SubmitEvent) {
		event.preventDefault()

		if (isCreating()) {
			return
		}

		setError('')
		setSuccess(false)
		setCreating(true)

		try {
			if (
				!name() ||
				!address() ||
				!room() ||
				!username() ||
				!password()
			) {
				setError('Missing params')
				return
			}

			let addr = address()
			if (!addr.includes(':')) {
				addr += ':' + DefaultServerPort
			}

			await state.createServer(client, {
				name: name(),
				address: addr,
				room: room(),
				username: username(),
				password: password(),
			})

			setSuccess(true)

			setName('')
			setAddress('')
			setRoom('')
			setUsername('')
			setPassword('')
		} catch (err) {
			if (err instanceof ConnectError) {
				setError(err.message)
			} else {
				console.error('failed to create server:', err)
				setError('Internal error, check console')
			}
		} finally {
			setCreating(false)
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
				<div class={stylesCommon.successMessage}>Created</div>
			</Show>

			<h1>Create Server</h1>

			<form onSubmit={submit} class={stylesCommon.form}>
				<table>
					<tbody>
						<tr>
							<td>
								<label for="create-server-name">Name</label>
							</td>
							<td>
								<input
									id="create-server-name"
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
								<label for="create-server-address">
									Address
								</label>
							</td>
							<td>
								<input
									id="create-server-address"
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
								<label for="create-server-room">Room</label>
							</td>
							<td>
								<input
									id="create-server-room"
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
								<label for="create-server-username">
									Username
								</label>
							</td>
							<td>
								<input
									id="create-server-username"
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
								<label for="create-server-password">
									Password
								</label>
							</td>
							<td>
								<input
									id="create-server-password"
									type="password"
									placeholder=""
									value={password()}
									onChange={(e) =>
										setPassword(e.currentTarget.value)
									}
									required={true}
								/>
							</td>
						</tr>
					</tbody>
				</table>

				<input
					type="submit"
					value="Create"
					disabled={isCreating()}
				/>
			</form>
		</div>
	)
}
