import styles from './UpdatePage.module.css'

import { Component, createSignal, Show } from 'solid-js'
import { useGlobalState } from '../ctx'

/**
 * The update page shows information about the current update and the latest update, if known.
 * It also allows for manually checking for updates.
 */
export const UpdatePage: Component = () => {
	const state = useGlobalState()

	const [isChecking, setChecking] = createSignal(false)
	const doCheck = async () => {
		setChecking(true)
		try {
			await state.checkForNewUpdate()
		} catch (err) {
			alert('Internal error, check console')
			console.error('failed to check for updates:', err)
		} finally {
			setChecking(false)
		}
	}

	return (
		<div class={styles.container}>
			<h1>Running FriendNet Client v{state.currentUpdate().version}</h1>

			<Show
				when={state.latestUpdate()}
				keyed={true}
				fallback={<h2>You are up-to-date.</h2>}
			>
				<Show
					when={state.latestUpdate()!.isValid}
					fallback={
						<div class={styles.invalid}>
							<h2>Invalid update signature!</h2>
							<p>
								The client checked for an update and found one,
								but its signature was invalid.
							</p>
							<p>
								This is indicative of either a misconfiguration
								of the FriendNet update system, or a malicious
								attempt to get you to download a potentially
								harmful update.
							</p>
							<p>Do not download this update.</p>
						</div>
					}
				>
					<div class={styles.new}>
						<h2>New update: v{state.latestUpdate()!.version}</h2>
						<pre class={styles.description}>
							{state.latestUpdate()!.description}
						</pre>
						<a href={state.latestUpdate()!.url} target="_blank">
							[Download]
						</a>
						{' '}
						<a href="https://friendnet.org/docs/client/updating/">
							[Updating Guide]
						</a>
					</div>
				</Show>
			</Show>

			<br />

			<Show
				when={isChecking()}
				fallback={<button onClick={doCheck}>Check for Update</button>}
			>
				<button disabled={true}>Checking...</button>
			</Show>
		</div>
	)
}
