import { Component, ErrorBoundary, JSX, Show } from 'solid-js'

import styles from './Layout.module.css'

import stopImg from '../asset/img/stop.svg'

import { useGlobalState, useRpcClient } from '../ctx'
import { AppName, TransfersOptionId } from '../constant'
import { ServerBrowser } from './ServerBrowser'
import { Previewer } from './Previewer'
import { A } from '@solidjs/router'

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
			setTimeout(() => window.location.assign('about:blank'), 100)
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

				<div class={styles.options}>
					<A
						href="/transfers"
						class={styles.option}
						id={TransfersOptionId}
					>
						⏳ Transfers
					</A>{' '}
					<A href="/settings" class={styles.option}>
						🔧 Client Settings
					</A>{' '}
					<A href="/logs" class={styles.option}>
						🔎 Log Viewer
					</A>{' '}
					<A href="/update" class={styles.option}>
						<span>📌 v{state.currentUpdate().version}</span>
						<Show when={state.latestUpdate()}>
							{' '}
							<Show when={state.latestUpdate()!.isValid}>
								<span class={styles.updateNew}>
									New update: v{state.latestUpdate()!.version}{' '}
									(click for info)
								</span>
							</Show>
							<Show when={!state.latestUpdate()!.isValid}>
								<span class={styles.updateInvalid}>
									Update signature invalid, do not update!
								</span>
							</Show>
						</Show>
					</A>
				</div>

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
						fallback={(err) => {
							console.log('failed to render page content:', err)

							return (
								<div class={styles.errorContainer}>
									<h1>
										Failed to render page content
									</h1>
									<div class={styles.error}>{err?.message ?? err}</div>
									<p>See console for details.</p>
								</div>
							)
						}}
					>
						{props.children}
					</ErrorBoundary>
				</div>
			</main>
		</div>
	)
}
