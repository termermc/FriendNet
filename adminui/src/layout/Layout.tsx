import { Component, ErrorBoundary, JSX, Show } from 'solid-js'

import styles from './Layout.module.css'

import { AppName } from '../constant'
import { A } from '@solidjs/router'
import { useServerInfo } from '../ctx'
import { RoomBrowser } from './RoomBrowser'

type LayoutProps = {
	children: JSX.Element
}

export const Layout: Component<LayoutProps> = (props) => {
	const serverInfo = useServerInfo()

	return (
		<div class={styles.container}>
			<header>
				<span class={styles.headerTitle}>{AppName}</span>

				<div class={styles.options}>
					<A href="/createroom" class={styles.option}>
						🚪 Create Room
					</A>{' '}
					<Show when={serverInfo.rpc!.allowedMethods.includes('*')}>
						<span
							title="Click for information"
							onClick={() =>
								alert(
									'The server RPC interface being used does not have wildcard permissions. Some functionality may not work.',
								)
							}
							classList={{
								[styles.option]: true,
								[styles.missingPermissions]: true,
							}}
						>
							⚠️ Missing Permissions
						</span>
					</Show>
				</div>
			</header>

			<main>
				<div class={styles.sidebar}>
					<RoomBrowser />
				</div>

				<div class={styles.content}>
					<ErrorBoundary
						fallback={(err) => {
							console.log('failed to render page content:', err)

							return (
								<div class={styles.errorContainer}>
									<h1>Failed to render page content</h1>
									<div class={styles.error}>
										{err?.message ?? err}
									</div>
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
