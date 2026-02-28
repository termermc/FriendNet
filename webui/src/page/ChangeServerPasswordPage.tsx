import { Component, createSignal, Show } from 'solid-js'

import stylesCommon from '../common.module.css'
import { useGlobalState } from '../ctx'
import { ConnectError } from '@connectrpc/connect'
import { useLocation, useParams } from '@solidjs/router'

const Page: Component = () => {
	const { uuid } = useParams<{ uuid: string }>()
	const state = useGlobalState()

	const server = state.getServerByUuid(uuid)
	if (!server) {
		return <h1>No such server "{uuid}"</h1>
	}

	const [curPass, setCurPass] = createSignal('')
	const [newPass, setNewPass] = createSignal('')

	const [error, setError] = createSignal('')
	const [isChanging, setChanging] = createSignal(false)
	const [isSuccess, setSuccess] = createSignal(false)
	const submit = async function (event: SubmitEvent) {
		event.preventDefault()

		if (isChanging()) {
			return
		}

		setError('')
		setSuccess(false)
		setChanging(true)

		try {
			if (!curPass() || !newPass()) {
				setError('Missing params')
				return
			}

			await server.changeAccountPassword(curPass(), newPass())

			setSuccess(true)

			setCurPass('')
			setNewPass('')
		} catch (err) {
			if (err instanceof ConnectError) {
				setError(err.message)
			} else {
				console.error('failed to update server:', err)
				setError('Internal error, check console')
			}
		} finally {
			setChanging(false)
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
				<div class={stylesCommon.successMessage}>Password Changed</div>
			</Show>

			<h1>Change Account Password For {server.name()}</h1>

			<form onSubmit={submit} class={stylesCommon.form}>
				<table>
					<tbody>
						<tr>
							<td>
								<label for="change-password-username">
									Username
								</label>
							</td>
							<td>
								<input
									id="change-password-username"
									type="text"
									placeholder=""
									value={server.username()}
									disabled={true}
								/>
							</td>
						</tr>

						<tr>
							<td>
								<label for="change-password-current">
									Current Password
								</label>
							</td>
							<td>
								<input
									id="change-password-current"
									type="password"
									placeholder=""
									value={curPass()}
									onChange={(e) =>
										setCurPass(e.currentTarget.value)
									}
									required={true}
								/>
							</td>
						</tr>

						<tr>
							<td>
								<label for="change-password-new">
									New Password
								</label>
							</td>
							<td>
								<input
									id="change-password-new"
									type="password"
									placeholder=""
									value={newPass()}
									onChange={(e) =>
										setNewPass(e.currentTarget.value)
									}
									required={true}
								/>
							</td>
						</tr>
					</tbody>
				</table>

				<input
					type="submit"
					value="Change Password"
					disabled={isChanging()}
				/>
			</form>
		</div>
	)
}

export const ChangeServerPasswordPage: Component = () => {
	const loc = useLocation()

	return (
		<Show when={loc.pathname} keyed>
			<Page />
		</Show>
	)
}
