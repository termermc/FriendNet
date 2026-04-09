import { Component, createSignal, Show } from 'solid-js'

import stylesCommon from '../common.module.css'
import { useNavigate } from '@solidjs/router'
import { ConnectError } from '@connectrpc/connect'
import { useAddRoom, useRpcClient } from '../ctx'

export const CreateRoomPage: Component = () => {
	const client = useRpcClient()
	const addRoom = useAddRoom()
	const navigate = useNavigate()

	const [name, setName] = createSignal('')
	const [isCreating, setCreating] = createSignal(false)
	const [error, setError] = createSignal('')
	const submit = async (e: Event) => {
		e.preventDefault()

		if (isCreating()) {
			return
		}

		try {
			setCreating(true)

			const { room } = await client.createRoom({
				name: name(),
			})

			addRoom(room!)
			navigate('/room/' + room!.name)
		} catch (err) {
			console.error('failed to create room:', err)

			if (err instanceof ConnectError) {
				setError(err.message)
				return
			}

			setError('Internal error, check console')
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
			<h1>Create Room</h1>

			<Show when={error()}>
				<div class={stylesCommon.errorMessage}>{error()}</div>
				<br/>
			</Show>

			<form class={stylesCommon.form} onSubmit={submit}>
				<table>
					<tbody>
						<tr>
							<td>
								<label for="room-name">Name</label>
							</td>
							<td>
								<input
									type="text"
									id="room-name"
									name="room-name"
									maxlength={16}
									value={name()}
									onInput={(e) =>
										setName(e.currentTarget.value)
									}
								/>
							</td>
						</tr>
					</tbody>
				</table>

				<input
					type="submit"
					value="Create Room"
					disabled={isCreating()}
				/>
			</form>
		</div>
	)
}
