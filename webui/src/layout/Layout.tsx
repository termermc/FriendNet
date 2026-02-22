import { Component, ErrorBoundary, JSX, Show } from 'solid-js'

import styles from './Layout.module.css'

import stopImg from '../asset/img/stop.svg'

import { useGlobalState, useRpcClient } from '../ctx'
import { AppName } from '../constant'
import { ServerBrowser } from './ServerBrowser'
import { Previewer } from './Previewer'

type LayoutProps = {
	children: JSX.Element
}

export const Layout: Component<LayoutProps> = (props) => {
	const state = useGlobalState()
	const client = useRpcClient()

	let isStopping = false
	async function stop() {
		if (isStopping) {
			return
		}

		if (!confirm('Are you sure you want to stop the client?')) {
			return
		}

		try {
			isStopping = true
			await client.stop({})
			window.close()
		} catch (err) {
			console.error('failed to stop client:', err)
			alert('Failed to stop client, see console for details')
		} finally {
			isStopping = false
		}
	}

	return (
		<div class={styles.container}>
			<header>
				<span class={styles.headerTitle}>{AppName}</span>

				<button
					class={styles.stopButton}
					title="Stop Client"
					onClick={stop}
				>
					<img src={stopImg} alt="stop" />
				</button>
			</header>

			<main>
				<div class={styles.sidebar}>
					<Show when={state.previewInfo()} keyed={true}>
						<Previewer info={state.previewInfo()!} />
					</Show>

					<ServerBrowser />
				</div>

				<div class={styles.content}>
					<ErrorBoundary
						fallback={
							<h1>
								Failed to render page content, check console
							</h1>
						}
					>
						{props.children}
					</ErrorBoundary>
				</div>
			</main>
		</div>
	)
}
